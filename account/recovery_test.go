package account

import (
	"context"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/pool/internal/test"
	"github.com/lightninglabs/pool/poolscript"
	"github.com/lightningnetwork/lnd/keychain"
)

// getInitialBatchKey returns the first batchkey value.
func getInitialBatchKey() *btcec.PublicKey {
	batchKey, _ := DecodeAndParseKey(
		"02824d0cbac65e01712124c50ff2cc74ce22851d7b444c1bf2ae66afefb" +
			"8eaf27f",
	)

	return batchKey
}

// getInitialBatchKey returns an auctioneer key for testing.
func getAuctioneerKey() *btcec.PublicKey {
	key, _ := DecodeAndParseKey(
		"02538d663e931822b1fa6af176c735036c09e9c0477fbcb884006177444" +
			"47cce23",
	)
	return key
}

// getSecret random (fixed) secret for testing.
func getSecret() [32]byte {
	return [32]byte{102, 97, 108, 99, 111, 110}
}

var findAccountTestCases = []struct {
	name             string
	config           RecoveryConfig
	traderKeys       []string
	expectedAccounts int
}{{
	name: "we are able to find the two initial accounts successfully",
	config: RecoveryConfig{
		AccountTarget:    2,
		InitialBatchKey:  getInitialBatchKey(),
		AuctioneerPubKey: getAuctioneerKey(),
		FirstBlock:       100,
		LastBlock:        200,
		Transactions:     []lndclient.Transaction{},
	},
	traderKeys: []string{
		"0214cd678a565041d00e6cf8d62ef8add33b4af4786fb2beb87b366a2e1" +
			"51fcee7",
		"027b27d419683ea2f58feff2da1a49c7defbddb77da0ab1e514c4c44961" +
			"c792d07",
	},
	expectedAccounts: 2,
}, {
	name: "if we already found the account target we stop early",
	config: RecoveryConfig{
		AccountTarget:    1,
		InitialBatchKey:  getInitialBatchKey(),
		AuctioneerPubKey: getAuctioneerKey(),
		FirstBlock:       100,
		LastBlock:        200,
		Transactions:     []lndclient.Transaction{},
	},
	traderKeys: []string{
		"0214cd678a565041d00e6cf8d62ef8add33b4af4786fb2beb87b366a2e1" +
			"51fcee7",
		"027b27d419683ea2f58feff2da1a49c7defbddb77da0ab1e514c4c44961" +
			"c792d07",
	},
	expectedAccounts: 1,
}}

// TestFindInitialAccountState checks that we are able to find the initial state
// for some lost accounts.
func TestFindInitialAccountState(t *testing.T) {
	for _, tc := range findAccountTestCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			possibleAccounts := make(
				map[*Account]struct{},
				len(tc.traderKeys),
			)

			for idx, tk := range tc.traderKeys {
				pubKey, _ := DecodeAndParseKey(tk)
				kd := &keychain.KeyDescriptor{
					KeyLocator: keychain.KeyLocator{
						Index: uint32(idx),
					},
					PubKey: pubKey,
				}

				acc := &Account{
					TraderKey:     kd,
					AuctioneerKey: tc.config.AuctioneerPubKey,
					Secret:        getSecret(),
				}
				possibleAccounts[acc] = struct{}{}

				script, _ := poolscript.AccountScript(
					177,
					acc.TraderKey.PubKey,
					tc.config.AuctioneerPubKey,
					poolscript.IncrementKey(
						tc.config.InitialBatchKey,
					),
					acc.Secret,
				)

				tc.config.Transactions = append(
					tc.config.Transactions,
					lndclient.Transaction{
						Tx: &wire.MsgTx{
							TxOut: []*wire.TxOut{
								{
									PkScript: script,
								},
							},
						},
					},
				)
			}

			accounts := findAccounts(tc.config, possibleAccounts)

			if len(accounts) != tc.expectedAccounts {
				t.Fatalf("number of accounts don't match, "+
					"got %d wanted %d",
					len(accounts), tc.expectedAccounts)
			}
		})
	}
}

// TestGenerateRecoveryKeys tests that a certain number of keys can be created
// for account recovery.
func TestGenerateRecoveryKeys(t *testing.T) {
	t.Parallel()

	walletKit := test.NewMockWalletKit()
	keys, err := GenerateRecoveryKeys(
		context.Background(), DefaultAccountKeyWindow, walletKit,
	)
	if err != nil {
		t.Fatalf("could not generate keys: %v", err)
	}

	if len(keys) != int(DefaultAccountKeyWindow) {
		t.Fatalf("unexpected number of keys, got %d wanted %d",
			len(keys), DefaultAccountKeyWindow)
	}
}
