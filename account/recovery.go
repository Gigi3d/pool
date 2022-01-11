package account

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/pool/poolscript"
	"github.com/lightningnetwork/lnd/keychain"
)

var (
	// DefaultAccountKeyWindow is the number of account keys that are
	// derived to be checked on recovery. This is the absolute maximum
	// number of accounts that can ever be restored. But the trader doesn't
	// necessarily make as many requests on recovery, if no accounts are
	// found for a certain number of tries.
	DefaultAccountKeyWindow uint32 = 500
)

// GetAuctioneerData returns the auctioner data for a given environment.
func GetAuctioneerData(network string) (string, uint32) {
	var auctioneerKey string
	var fstBlock uint32

	switch network {
	case "mainnet":
		auctioneerKey = "028e87bdd134238f8347f845d9ecc827b843d0d1e2" +
			"7cdcb46da704d916613f4fce"
		fstBlock = 648168
	case "testnet":
		auctioneerKey = "025dea8f5c67fb3bdfffb3123d2b7045dc0a3c75e8" +
			"22fabb39eb357480e64c4a8a"
		fstBlock = 1834898
	}
	return auctioneerKey, fstBlock
}

// DecodeAndParseKey decode and parse a btc public key.
func DecodeAndParseKey(key string) (*btcec.PublicKey, error) {
	kStr, err := hex.DecodeString(key)
	if err != nil {
		return nil, err
	}
	k, err := btcec.ParsePubKey(kStr, btcec.S256())
	if err != nil {
		return nil, err
	}
	return k, nil
}

// RecoveryConfig contains all of the required dependencies for carrying out the
// recovery process duties.
type RecoveryConfig struct {
	// Network to run recovery on.
	Network string

	// Number of accounts that we are trying to find.
	AccountTarget uint32

	// FirstBlock block marks the initial height for our search.
	FirstBlock uint32

	// LastBlock block marks the final height for our search.
	LastBlock uint32

	// Transactions
	Transactions []lndclient.Transaction

	// Signer
	Signer lndclient.SignerClient

	// Wallet
	Wallet lndclient.WalletKitClient

	/*
		Auctioneer data
	*/

	// Initial value for the batch key.
	InitialBatchKey *btcec.PublicKey

	// Auctioneer public key.
	AuctioneerPubKey *btcec.PublicKey
}

// RecoverAccounts tries to recover valid accounts using the given configuration.
func RecoverAccounts(ctx context.Context, cfg RecoveryConfig) (
	[]*Account, error) {

	accounts, err := recoverInitalState(ctx, cfg)
	if err != nil {
		return nil, err
	}

	accounts = updateAccountStates(cfg, accounts)

	return accounts, nil
}

// recoverInitalState finds accounts in their initial state (creation).
func recoverInitalState(ctx context.Context, cfg RecoveryConfig) (
	[]*Account, error) {

	log.Debugf(
		"Recovering initial states for %d accounts...",
		cfg.AccountTarget,
	)

	var accounts []*Account

	possibleAccounts, err := recreatePossibleAccounts(ctx, cfg)
	if err != nil {
		return accounts, err
	}

	accounts = findAccounts(cfg, possibleAccounts)

	log.Debugf(
		"Found initial tx for %d/%d accounts", len(accounts),
		cfg.AccountTarget,
	)

	return accounts, nil
}

// recreatePossibleAccounts returns a set of potentially valid accounts
// by generating a set of recovery keys.
func recreatePossibleAccounts(ctx context.Context, cfg RecoveryConfig) (
	map[*Account]struct{}, error) {

	possibleAccounts := make(map[*Account]struct{}, cfg.AccountTarget)

	// Prepare the keys we are going to try. Possibly not all of them will
	// be used.
	KeyDescriptors, err := GenerateRecoveryKeys(
		ctx, cfg.AccountTarget, cfg.Wallet,
	)
	if err != nil {
		return possibleAccounts, fmt.Errorf(
			"error generating keys: %v", err,
		)
	}

	for _, keyDes := range KeyDescriptors {
		secret, err := cfg.Signer.DeriveSharedKey(
			ctx, cfg.AuctioneerPubKey, &keyDes.KeyLocator,
		)
		if err != nil {
			return possibleAccounts, fmt.Errorf(
				"error deriving shared key: %v", err,
			)
		}

		acc := &Account{
			TraderKey:     keyDes,
			AuctioneerKey: cfg.AuctioneerPubKey,
			Secret:        secret,
		}
		possibleAccounts[acc] = struct{}{}
	}

	return possibleAccounts, nil
}

// findAccounts tries to find on-chain footprint for the creation of a set
// of possible pool accounts.
func findAccounts(cfg RecoveryConfig,
	possibleAccounts map[*Account]struct{}) []*Account {

	// The process looks like:
	//     - Fix the batch counter.
	//     - Fix a block height.
	//     - Recreate the script for each of the possible accounts.
	//         - Look for the script in all of the node txs.

	target := cfg.AccountTarget
	accounts := make([]*Account, 0, target)

	batchKey := cfg.InitialBatchKey
	// DecrementKey so the first for iteration starts with the
	// first batchKey value.
	batchKey = poolscript.DecrementKey(batchKey)

searchLoop:
	for batchCounter := 0; batchCounter < 5000; batchCounter++ {
		batchKey = poolscript.IncrementKey(batchKey)
		for h := cfg.FirstBlock; h < cfg.LastBlock; h++ {
		nextAccount:
			for acc := range possibleAccounts {
				script, err := poolscript.AccountScript(
					h,
					acc.TraderKey.PubKey,
					cfg.AuctioneerPubKey,
					batchKey,
					acc.Secret,
				)
				if err != nil {
					key := acc.TraderKey.PubKey
					log.Debugf(
						"unable to generate script: \n"+
							"height: %v\n"+
							"batchKey: %v\n"+
							"traderKey: %v\n",
						h,
						batchKey,
						hex.EncodeToString(
							key.SerializeCompressed(),
						),
					)
					goto nextAccount
				}

				if tx, idx := appearsInTxs(
					acc, script, cfg.Transactions,
				); tx != nil {
					// If it's a match, populate
					// the account information.
					acc.Expiry = h
					acc.Value = btcutil.Amount(
						tx.Tx.TxOut[idx].Value,
					)
					acc.BatchKey = batchKey
					acc.OutPoint = wire.OutPoint{
						Hash:  tx.Tx.TxHash(),
						Index: idx,
					}
					acc.LatestTx = tx.Tx
					acc.State = StateOpen

					accounts = append(accounts, acc)

					delete(possibleAccounts, acc)

					// If we already found all the
					// accounts that we were looking
					// for quit the search.
					if len(accounts) == int(target) {
						break searchLoop
					}
				}
			}
		}
	}

	return accounts
}

// appearsInTxs tries to locate a script in any of the provided transactions.
func appearsInTxs(acc *Account, script []byte, txs []lndclient.Transaction) (
	*lndclient.Transaction, uint32) {

	for _, tx := range txs {
		idx, ok := poolscript.LocateOutputScript(tx.Tx, script)
		if ok {
			tx := tx // make scopelint happy
			traderKey := acc.TraderKey.PubKey.SerializeCompressed()
			log.Debugf(
				"found accout with trader key %v",
				hex.EncodeToString(traderKey),
			)
			return &tx, idx
		}
	}
	return nil, 0
}

// updateAccountStates tries to update the states for every provided
// account up to their latest state by following the on chain
// modification footprints.
func updateAccountStates(cfg RecoveryConfig, accounts []*Account) []*Account {

	recoveredAccounts := make([]*Account, 0, len(accounts))

nextAccount:
	for _, acc := range accounts {
		log.Debugf(
			"Updating state for account with trader key %v",
			hex.EncodeToString(
				acc.TraderKey.PubKey.SerializeCompressed(),
			),
		)
		for _, tx := range cfg.Transactions {
			if poolscript.IncludesPreviousOutPoint(
				tx.Tx, acc.OutPoint,
			) {
				newAcc, err := findAccountUpdate(
					cfg, acc, tx.Tx,
				)
				if err != nil {
					log.Debugf(
						"unable to find account update "+
							"%v",
						err,
					)
					continue nextAccount
				}
				acc = newAcc
			}
		}
		recoveredAccounts = append(
			recoveredAccounts, acc,
		)
		log.Debugf(
			"latest state for account:\n"+
				"    Trader key: %v\n"+
				"    Amount: %v\n"+
				"    Expiry: %v\n",
			hex.EncodeToString(
				acc.TraderKey.PubKey.SerializeCompressed(),
			),
			acc.Value,
			acc.Expiry,
		)
	}
	return recoveredAccounts
}

// findAccountUpdate tries to find the new account values after an update.
func findAccountUpdate(cfg RecoveryConfig, acc *Account, tx *wire.MsgTx) (
	*Account, error) {

	newAcc := acc.Copy()
	newAcc.BatchKey = poolscript.IncrementKey(newAcc.BatchKey)

	idx, ok := matchScript(newAcc, newAcc.Expiry, tx)
	if ok {
		newAcc.Value = btcutil.Amount(tx.TxOut[idx].Value)
		newAcc.OutPoint = wire.OutPoint{
			Hash:  tx.TxHash(),
			Index: idx,
		}
		newAcc.LatestTx = tx

		return newAcc, nil
	}

	// If the update included a new expiration date we need to brute force
	// our new expiration date again.
	for height := cfg.FirstBlock; height <= cfg.LastBlock; height++ {
		idx, ok := matchScript(newAcc, height, tx)
		if ok {
			newAcc.Expiry = height
			newAcc.Value = btcutil.Amount(
				tx.TxOut[idx].Value,
			)
			newAcc.OutPoint = wire.OutPoint{
				Hash:  tx.TxHash(),
				Index: idx,
			}
			newAcc.LatestTx = tx
			return newAcc, nil
		}
	}

	return nil, fmt.Errorf("account update not found")
}

// matchScript creates a new Account script and tries to locate the output in a
// transaction.
func matchScript(acc *Account, expiry uint32, tx *wire.MsgTx) (uint32, bool) {
	script, err := poolscript.AccountScript(
		expiry,
		acc.TraderKey.PubKey,
		acc.AuctioneerKey,
		acc.BatchKey,
		acc.Secret,
	)
	if err != nil {
		log.Debugf("%v", err)
		return 0, false
	}

	return poolscript.LocateOutputScript(tx, script)
}

// GenerateRecoveryKeys generates a list of key descriptors for all possible
// keys that could be used for trader accounts, up to a hard coHashded limit.
func GenerateRecoveryKeys(ctx context.Context, accountTarget uint32,
	wallet lndclient.WalletKitClient) (
	[]*keychain.KeyDescriptor, error) {

	acctKeys := make([]*keychain.KeyDescriptor, accountTarget)
	for i := uint32(0); i < accountTarget; i++ {
		key, err := wallet.DeriveKey(ctx, &keychain.KeyLocator{
			Family: poolscript.AccountKeyFamily,
			Index:  i,
		})
		if err != nil {
			return nil, err
		}

		acctKeys[i] = key
	}
	return acctKeys, nil
}
