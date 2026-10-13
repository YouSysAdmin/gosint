// Gosint, Copyright (c) 2026 YouSysAdmin

// Package crypto holds the transforms that start from a crypto wallet.
package crypto

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"math/big"
	"net/url"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// EtherscanURL is the Etherscan v2 API.
const EtherscanURL = "https://api.etherscan.io/v2/api"

// Etherscan is the Etherscan v2 account API.
type Etherscan struct {
	// BaseURL replaces the ETHERSCAN_API_URL parameter, for tests.
	BaseURL string
}

var etherscanParams = []transform.Param{
	{Name: "ETHERSCAN_API_KEY", Type: transform.ParamSecret, Required: true, Description: "The Etherscan API key."},
	{Name: "ETHERSCAN_API_URL", Type: transform.ParamURL, Default: EtherscanURL, Description: "The Etherscan API URL."},
}

// call asks the account module for action on address.
func (e Etherscan) call(ctx context.Context, x *transform.Exec, action, address, offset string, out any) error {
	base := x.Param("ETHERSCAN_API_URL")
	if e.BaseURL != "" {
		base = e.BaseURL
	}

	q := url.Values{"chainid": {"1"}, "module": {"account"}, "action": {action}, "address": {address},
		"startblock": {"0"}, "endblock": {"99999999"}, "page": {"1"}, "offset": {offset}, "sort": {"asc"},
		"apikey": {x.Param("ETHERSCAN_API_KEY")}}

	var answer struct {
		Status  string         `json:"status"`
		Message string         `json:"message"`
		Result  jsontext.Value `json:"result"`
	}

	// The URL is a parameter, so the guarded client.
	if _, err := httpx.GetJSONWeb(ctx, x, withQuery(base, q), nil, &answer); err != nil {
		return err
	}

	if answer.Status != "1" {
		if answer.Message == "No transactions found" {
			return nil
		}

		return fmt.Errorf("etherscan: %s", answer.Message)
	}

	return json.Unmarshal(answer.Result, out)
}

// CryptoWalletToTransactions adds a wallet's transactions.
type CryptoWalletToTransactions struct{ Etherscan }

// Meta describes the transform.
func (CryptoWalletToTransactions) Meta() transform.Meta {
	return transform.Meta{Name: "cryptowallet_to_transactions", Category: "Crypto wallet", Input: "cryptowallet",
		Output: "cryptowallettransaction", Params: etherscanParams,
		Description: "List the first 100 Ethereum transactions of a wallet (Etherscan)."}
}

// Run enriches each wallet. A transaction is a node between the two
// wallets, not an edge: one edge per wallet pair would fold every
// transaction between them into one.
func (e CryptoWalletToTransactions) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		addr := props.Str(in.Properties, "address")

		var txs []map[string]string
		if err := e.call(ctx, x, "txlist", addr, "100", &txs); err != nil {
			x.Warn("%s: %v", addr, err)

			continue
		}

		for _, t := range txs {
			to := cmp.Or(t["to"], t["contractAddress"], addr)
			p := map[string]any{
				"hash": t["hash"], "currency": "ETH", "source": map[string]any{"address": t["from"]}, "target": map[string]any{"address": to},
				"value": weiToEth(t["value"]), "timestamp": t["timeStamp"], "block_number": t["blockNumber"],
				"block_hash": t["blockHash"], "nonce": t["nonce"], "transaction_index": t["transactionIndex"],
				"gas": t["gas"], "gas_price": t["gasPrice"], "gas_used": t["gasUsed"],
				"cumulative_gas_used": t["cumulativeGasUsed"], "input": t["input"], "contract_address": t["contractAddress"],
			}

			// An absent field arrives as "", which a numeric field refuses.
			for k, v := range p {
				if v == "" {
					delete(p, k)
				}
			}

			tx, err := x.Node(ctx, "cryptowallettransaction", p)
			if err != nil {
				x.Warn("%s: %v", t["hash"], err)

				continue
			}

			from, ferr := x.Node(ctx, "cryptowallet", map[string]any{"address": t["from"]})
			dst, terr := x.Node(ctx, "cryptowallet", map[string]any{"address": to})
			if ferr != nil || terr != nil {
				continue
			}

			_ = x.Link(ctx, from, tx, "SENT")
			_ = x.Link(ctx, tx, dst, "RECEIVED_BY")
		}

		x.Info("%s: %d transactions", addr, len(txs))
	}

	return nil
}

// weiToEth renders a wei amount as ether, exactly.
func weiToEth(wei string) float64 {
	v, ok := new(big.Float).SetString(wei)
	if !ok {
		return 0
	}

	eth, _ := new(big.Float).Quo(v, big.NewFloat(1e18)).Float64()

	return eth
}

// CryptoWalletToNFTs adds the NFTs a wallet traded.
type CryptoWalletToNFTs struct{ Etherscan }

// Meta describes the transform.
func (CryptoWalletToNFTs) Meta() transform.Meta {
	return transform.Meta{Name: "cryptowallet_to_nfts", Category: "Crypto wallet", Input: "cryptowallet",
		Output: "cryptonft", Params: etherscanParams,
		Description: "List the NFT transfers of a wallet (Etherscan)."}
}

// Run enriches each wallet.
func (e CryptoWalletToNFTs) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		addr := props.Str(in.Properties, "address")

		var nfts []map[string]string
		if err := e.call(ctx, x, "tokennfttx", addr, "10000", &nfts); err != nil {
			x.Warn("%s: %v", addr, err)

			continue
		}

		for _, n := range nfts {
			nft := map[string]any{"wallet": map[string]any{"address": addr}, "contract_address": n["contractAddress"],
				"chain": "ethereum", "token_id": n["tokenID"], "collection_name": n["tokenName"], "name": n["tokenSymbol"]}
			if _, err := x.NodeAndLink(ctx, in, "OWNS", "cryptonft", nft); err != nil {
				x.Warn("%s: %v", addr, err)
			}
		}
	}

	return nil
}

// All is the package's transforms.
func All() []transform.Transform {
	return []transform.Transform{CryptoWalletToTransactions{}, CryptoWalletToNFTs{}}
}

// withQuery adds q to base, keeping a query base already carries.
func withQuery(base string, q url.Values) string {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}

	return base + sep + q.Encode()
}
