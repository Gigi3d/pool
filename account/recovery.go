package account

import (
	"context"
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec"
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

	accounts, err = updateAccountStates(cfg, accounts)
	if err != nil {
		return nil, err
	}

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

	// TODO (positiveblue): recover initial state

	log.Debugf(
		"Found initial tx for %d/%d accounts", len(accounts),
		cfg.AccountTarget,
	)

	return accounts, nil
}

// updateAccountStates tries to update the states for every provided
// account up to their latest state by following the on chain
// modification footprints.
func updateAccountStates(cfg RecoveryConfig, accounts []*Account) (
	[]*Account, error) {

	recoveredAccounts := make([]*Account, 0, len(accounts))

	// TODO (positiveblue): update account states

	return recoveredAccounts, nil
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
