// Gosint, Copyright (c) 2026 YouSysAdmin

package nucleireport_test

import (
	"slices"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/nucleireport"
)

// Lines as nuclei v3 writes them with -jsonl: an http match with a CVE,
// a network one on host:port, and a line that is not a result.
const jsonl = `{"template-id":"CVE-2021-44228","info":{"name":"Apache Log4j2 - RCE","author":["x"],"tags":["cve","rce","log4j"],"description":"Log4Shell.","reference":["https://logging.apache.org/log4j/2.x/security.html"],"severity":"critical","classification":{"cve-id":["cve-2021-44228"],"cwe-id":["cwe-502"],"cvss-score":10}},"type":"http","host":"https://app.example.com","port":"443","url":"https://app.example.com","matched-at":"https://app.example.com/login","extracted-results":["jndi:ldap"],"ip":"203.0.113.7","timestamp":"2026-10-06T10:00:00.123456Z","matcher-name":"dns-interaction"}
not json at all
{"template-id":"openssh-detect","info":{"name":"OpenSSH Service - Detect","tags":"network,ssh,detect","severity":"info","classification":{"cve-id":null,"cwe-id":null}},"type":"network","host":"app.example.com:22","port":22,"matched-at":"app.example.com:22","extracted-results":["SSH-2.0-OpenSSH_8.9p1"],"ip":"203.0.113.7","timestamp":"2026-10-06T10:00:01Z"}
`

func TestParseReadsJSONLines(t *testing.T) {
	rep, err := nucleireport.Parse([]byte(jsonl))
	if err != nil {
		t.Fatal(err)
	}

	if len(rep.Results) != 2 || rep.Skipped != 1 {
		t.Fatalf("results %d, skipped %d", len(rep.Results), rep.Skipped)
	}

	log4j, ssh := rep.Results[0], rep.Results[1]
	if log4j.Severity != "critical" || !slices.Equal(log4j.CVEs, []string{"CVE-2021-44228"}) || !slices.Equal(log4j.CWEs, []string{"CWE-502"}) ||
		log4j.CVSS != 10 || log4j.MatchedAt != "https://app.example.com/login" || log4j.Timestamp.IsZero() || len(log4j.References) != 1 {
		t.Errorf("log4j = %+v", log4j)
	}

	if ssh.Port != "22" || !slices.Equal(ssh.Tags, []string{"network", "ssh", "detect"}) || len(ssh.CVEs) != 0 || ssh.Extracted[0] != "SSH-2.0-OpenSSH_8.9p1" {
		t.Errorf("ssh = %+v", ssh)
	}
}

func TestParseReadsAJSONExport(t *testing.T) {
	rep, err := nucleireport.Parse([]byte(`[{"template-id":"git-config","info":{"name":"Git Config - Detect","severity":"medium"},"type":"http","host":"https://a.example","matched-at":"https://a.example/.git/config"}]`))
	if err != nil || len(rep.Results) != 1 || rep.Results[0].Name != "Git Config - Detect" {
		t.Fatalf("export = %+v, %v", rep, err)
	}

	if _, err := nucleireport.Parse([]byte("<nmaprun/>")); err == nil {
		t.Error("an nmap report read as nuclei")
	}
}
