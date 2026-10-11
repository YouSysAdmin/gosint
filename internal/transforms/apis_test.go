// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"encoding/json/v2"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/transforms/automation"
	"github.com/yousysadmin/gosint/internal/transforms/crypto"
	"github.com/yousysadmin/gosint/internal/transforms/ip"
	"github.com/yousysadmin/gosint/internal/transforms/phone"
)

func TestFraudScore(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/v3/me/": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key") != "k" {
				w.WriteHeader(http.StatusForbidden)

				return
			}

			_, _ = w.Write([]byte(`{"scamalytics":{"status":"ok","scamalytics_score":87,"scamalytics_risk":"high",
				"scamalytics_proxy":{"is_vpn":true}}}`))
		},
	})

	addr := h.node("ip", map[string]any{"address": "203.0.113.9"})
	h.run(ip.IPToFraudScore{BaseURL: strings.TrimSuffix(base, "/")},
		map[string]string{"SCAMLYTICS_USERNAME": "me", "SCAMLYTICS_API_KEY": "k"}, addr)
	h.expectEdges("203.0.113.9 -HAS_RISK_PROFILE-> 203.0.113.9 - Risk: high")
}

func TestPhoneCarrierInPlace(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"carrier":"Orange","country":"France"}`))
		},
	})

	n := h.node("phone", map[string]any{"number": "+33612345678"})
	h.run(phone.PhoneToCarrier{BaseURL: base}, map[string]string{"VERIPHONE_API_KEY": "k"}, n)

	got, err := h.st.Graph.Node(t.Context(), h.sketch, n.ID)
	if err != nil || got.Properties["carrier"] != "Orange" {
		t.Errorf("phone after = %+v %v", got, err)
	}
}

func TestEtherscanTransactionsAreNodes(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[
				{"hash":"0xaa","from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","value":"1500000000000000000","timeStamp":"1700000000","blockNumber":"1"},
				{"hash":"0xbb","from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","value":"1","timeStamp":"1700000001","blockNumber":"2"}]}`))
		},
	})

	w := h.node("cryptowallet", map[string]any{"address": "0x1111111111111111111111111111111111111111"})
	h.run(crypto.CryptoWalletToTransactions{BaseURL: base}, map[string]string{"ETHERSCAN_API_KEY": "k"}, w)

	nodes, _, err := h.st.Graph.Graph(t.Context(), h.sketch)
	if err != nil {
		t.Fatal(err)
	}

	txs := 0
	for _, n := range nodes {
		if n.Type == "cryptowallettransaction" {
			txs++
			if n.Label == "0xaa" && n.Properties["value"] != 1.5 {
				t.Errorf("value of 0xaa = %v, want 1.5 ether", n.Properties["value"])
			}
		}
	}

	// Two transactions between the same wallets stay two.
	if txs != 2 {
		t.Errorf("%d transaction nodes, want 2: %q log %q", txs, h.edges(), h.log)
	}
}

func TestN8NReceivesTheEntities(t *testing.T) {
	h := newHarness(t)
	var got map[string]any
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/hook": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &got)
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			_, _ = w.Write([]byte(`{"ok":true}`))
		},
	})

	h.run(automation.N8NConnector{}, map[string]string{"webhook_url": base + "hook", "N8N_WEBHOOK_TOKEN": "tok", "extra_payload": `{"case":"x"}`},
		h.node("domain", map[string]any{"domain": "example.com"}))

	inputs, _ := got["inputs"].([]any)
	if got["case"] != "x" || len(inputs) != 1 {
		t.Errorf("webhook got %v", got)
	}
}
