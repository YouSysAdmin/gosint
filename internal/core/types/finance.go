// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"regexp"
	"strings"
)

var (
	ethAddressRe    = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)
	btcLegacyRe     = regexp.MustCompile(`^[13][a-km-zA-HJ-NP-Z1-9]{25,34}$`)
	btcSegwitRe     = regexp.MustCompile(`^bc1[a-z0-9]{39,59}$`)
	cardSeparatorsR = strings.NewReplacer(" ", "", "-", "")
)

// currencyRes tell a wallet's currency from the shape of its address,
// the unambiguous ones only. An address matching none keeps whatever
// currency the person gave.
var currencyRes = []struct {
	re       *regexp.Regexp
	currency string
}{
	{ethAddressRe, "ETH"},
	{btcLegacyRe, "BTC"},
	{btcSegwitRe, "BTC"},
	{regexp.MustCompile(`^(?:[LM][1-9A-HJ-NP-Za-km-z]{26,33}|ltc1[a-z0-9]{39,59})$`), "LTC"},
	{regexp.MustCompile(`^D[5-9A-HJ-NP-U][1-9A-HJ-NP-Za-km-z]{32}$`), "DOGE"},
	{regexp.MustCompile(`^T[1-9A-HJ-NP-Za-km-z]{33}$`), "TRX"},
	{regexp.MustCompile(`^[48][1-9A-HJ-NP-Za-km-z]{94}$`), "XMR"},
	{regexp.MustCompile(`^r[1-9A-HJ-NP-Za-km-z]{24,34}$`), "XRP"},
}

// guessCurrency is the currency an address shape gives away, "" when
// the shape says nothing.
func guessCurrency(address string) string {
	for _, c := range currencyRes {
		if c.re.MatchString(address) {
			return c.currency
		}
	}

	return ""
}

// nonNegative refuses a negative number in any of the named fields.
func nonNegative(typeName, message string, p map[string]any, names ...string) error {
	for _, n := range names {
		if f, ok := toFloat(p[n]); ok && p[n] != nil && f < 0 {
			return fieldErr(typeName, n, "%s", message)
		}
	}

	return nil
}

// walletAddress is the address of a nested wallet.
func walletAddress(p map[string]any, key string) string {
	w, _ := p[key].(map[string]any)

	return str(w, "address")
}

func financeTypes() []spec {
	return []spec{
		{
			name: "bankaccount", title: "Bank Account", category: "financial", icon: "Landmark", color: "#D4B030",
			label: func(p map[string]any) string {
				var parts []string
				if truthy(p["bank_name"]) {
					parts = append(parts, pyStr(p["bank_name"]))
				}

				return strings.Join(append(parts, masked(str(p, "account_number"))), " - ")
			},
			fromString: into("account_number"),
		},
		{
			name: "creditcard", title: "Credit Card", category: "financial", icon: "CreditCard", color: "#285E8E",
			before: creditCardBefore,
			label: func(p map[string]any) string {
				var parts []string
				if truthy(p["card_type"]) {
					parts = append(parts, pyStr(p["card_type"]))
				}

				return strings.Join(append(parts, masked(str(p, "card_number"))), " ")
			},
			fromString: into("card_number"),
			detect: func(s string) bool {
				digits := cardSeparatorsR.Replace(s)
				if !allDigits(digits) || len(digits) < 13 || len(digits) > 19 {
					return false
				}

				return luhn(digits)
			},
		},
		{
			name: "cryptowallet", title: "Crypto Wallet", category: "crypto", icon: "Wallet", color: "#D4B030",
			after: func(p map[string]any) error {
				v := pyStrip(str(p, "address"))
				if v == "" {
					return fieldErr("cryptowallet", "address", "wallet address cannot be empty")
				}

				p["address"] = v
				if c := strings.ToUpper(pyStrip(str(p, "currency"))); c != "" {
					p["currency"] = c
				} else if c := guessCurrency(v); c != "" {
					p["currency"] = c
				}

				return nil
			},
			label: func(p map[string]any) string {
				if truthy(p["currency"]) {
					return str(p, "currency") + " " + str(p, "address")
				}

				return str(p, "address")
			},
			fromString: into("address"),
			detect: func(s string) bool {
				return len(s) >= 26 && guessCurrency(s) != ""
			},
		},
		{
			name: "cryptowallettransaction", title: "Crypto Transaction", category: "crypto", icon: "ArrowRightLeft", color: "#BFA750",
			after: func(p map[string]any) error {
				// The currency is the wallets', unless given.
				if c := strings.ToUpper(pyStrip(str(p, "currency"))); c != "" {
					p["currency"] = c
				} else if src, _ := p["source"].(map[string]any); truthy(src["currency"]) {
					p["currency"] = src["currency"]
				}

				if err := nonNegative("cryptowallettransaction", "monetary amounts must be non-negative", p,
					"value", "amount", "amount_usd"); err != nil {
					return err
				}

				return nonNegative("cryptowallettransaction", "integer values must be non-negative", p,
					"gas", "gas_price", "gas_used", "cumulative_gas_used", "block_number", "nonce",
					"transaction_index", "confirmations", "hop")
			},
			label: func(p map[string]any) string {
				if truthy(p["hash"]) {
					return pyStr(p["hash"])
				}

				src, dst := walletAddress(p, "source"), walletAddress(p, "target")
				if src == "" {
					return ""
				}

				if dst != "" {
					return "Transaction from " + firstRunes(src, 8) + "... to " + firstRunes(dst, 8) + "..."
				}

				return "Transaction from " + firstRunes(src, 8) + "..."
			},
		},
		{
			name: "cryptonft", title: "NFT", category: "crypto", icon: "Image", color: "#A5BF50",
			after: func(p map[string]any) error {
				contract := pyStrip(str(p, "contract_address"))
				if contract == "" {
					return fieldErr("cryptonft", "contract_address", "contract address cannot be empty")
				}

				if !ethAddressRe.MatchString(contract) {
					return fieldErr("cryptonft", "contract_address",
						"invalid contract address format, expected 0x followed by 40 hex characters")
				}

				token := pyStrip(str(p, "token_id"))
				if token == "" {
					return fieldErr("cryptonft", "token_id", "token ID cannot be empty")
				}

				p["contract_address"], p["token_id"] = contract, token
				if !truthy(p["chain"]) {
					// An ERC-721 or 1155 contract address, which the check above
					// insists on, lives on an EVM chain, Ethereum unless said.
					p["chain"] = "ethereum"
				}

				return nil
			},
			label: func(p map[string]any) string {
				switch {
				case truthy(p["name"]):
					return pyStr(p["name"])
				case truthy(p["collection_name"]):
					return pyStr(p["collection_name"]) + " #" + str(p, "token_id")
				default:
					return str(p, "contract_address") + ":" + str(p, "token_id")
				}
			},
		},
	}
}

// creditCardBefore keeps the number as digits, refuses one whose check
// digit is wrong, and names the card scheme from the number when the
// investigator has not. A number with masked digits (4111 **** ...) is
// kept as typed and not checked.
func creditCardBefore(p map[string]any) error {
	raw := pyStrip(str(p, "card_number"))
	if raw == "" {
		return nil
	}

	digits := cardSeparatorsR.Replace(raw)
	if !allDigits(digits) {
		return nil
	}

	if len(digits) < 12 || len(digits) > 19 {
		return fieldErr("creditcard", "card_number", "must be 12 to 19 digits")
	}

	if !luhn(digits) {
		return fieldErr("creditcard", "card_number", "fails the Luhn check, a digit is wrong")
	}

	p["card_number"] = digits
	if !truthy(p["card_type"]) {
		if brand := cardBrand(digits); brand != "" {
			p["card_type"] = brand
		}
	}

	return nil
}

// cardBrand is the scheme a card number's leading digits belong to.
func cardBrand(d string) string {
	lead := func(n int) int {
		if len(d) < n {
			return -1
		}

		v := 0
		for _, c := range d[:n] {
			v = v*10 + int(c-'0')
		}

		return v
	}

	in := func(v, lo, hi int) bool { return v >= lo && v <= hi }
	switch {
	case in(lead(2), 34, 34), in(lead(2), 37, 37):
		return "American Express"
	case in(lead(4), 3528, 3589):
		return "JCB"
	case in(lead(3), 300, 305), in(lead(2), 36, 36), in(lead(2), 38, 39):
		return "Diners Club"
	case in(lead(4), 6011, 6011), in(lead(3), 644, 649), in(lead(2), 65, 65):
		return "Discover"
	case in(lead(4), 2221, 2720), in(lead(2), 51, 55):
		return "Mastercard"
	case in(lead(1), 4, 4):
		return "Visa"
	case in(lead(2), 62, 62):
		return "UnionPay"
	case in(lead(2), 50, 50), in(lead(2), 56, 69):
		return "Maestro"
	}

	return ""
}
