// Gosint, Copyright (c) 2026 YouSysAdmin

// Package nmapreport reads nmap's XML output (nmap -oX), whether it
// came from the scanner run by a transform or a file an operator
// brings in.
package nmapreport

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Report is the hosts of one run.
type Report struct {
	Hosts []Host
}

// Host is one scanned host with an IP address. Hosts known only by a
// MAC address are left out.
type Host struct {
	Addr string

	// Hostnames are the names nmap found, PTR records and the name the
	// scan was given.
	Hostnames []Hostname
	Ports     []Port
}

// Hostname is one name of a host. Kind is nmap's: "PTR" for a reverse
// lookup, "user" for the name the scan was given.
type Hostname struct {
	Name string
	Kind string
}

// Port is one port of the report, in any state.
type Port struct {
	Number   int
	Protocol string
	State    string
	Service  string
	Product  string
	Version  string
}

// Open is the host's ports that may be open: "open", and the
// "open|filtered" a UDP scan reports when nothing answered, which is
// how most open UDP ports look. Closed and filtered ones are left out.
func (h Host) Open() []Port {
	var out []Port
	for _, p := range h.Ports {
		if p.State == "open" || p.State == "open|filtered" {
			out = append(out, p)
		}
	}

	return out
}

// Banner is the service's product and version as one line.
func (p Port) Banner() string {
	return strings.TrimSpace(p.Product + " " + p.Version)
}

// run is the part of nmap's XML read.
type run struct {
	XMLName xml.Name `xml:"nmaprun"`
	Hosts   []struct {
		Addresses []struct {
			Addr string `xml:"addr,attr"`
			Type string `xml:"addrtype,attr"`
		} `xml:"address"`
		Hostnames []struct {
			Name string `xml:"name,attr"`
			Type string `xml:"type,attr"`
		} `xml:"hostnames>hostname"`
		Ports []struct {
			Protocol string `xml:"protocol,attr"`
			ID       int    `xml:"portid,attr"`
			State    struct {
				State string `xml:"state,attr"`
			} `xml:"state"`
			Service struct {
				Name    string `xml:"name,attr"`
				Product string `xml:"product,attr"`
				Version string `xml:"version,attr"`
			} `xml:"service"`
		} `xml:"ports>port"`
	} `xml:"host"`
}

// Parse reads a report. Anything that is not an nmaprun document is
// refused.
func Parse(data []byte) (Report, error) {
	var r run
	if err := xml.Unmarshal(data, &r); err != nil {
		return Report{}, fmt.Errorf("not an nmap XML report: %w", err)
	}

	var report Report
	for _, h := range r.Hosts {
		host := Host{}
		for _, a := range h.Addresses {
			if a.Type == "ipv4" || a.Type == "ipv6" {
				host.Addr = a.Addr

				break
			}
		}

		if host.Addr == "" {
			continue
		}

		for _, n := range h.Hostnames {
			if name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(n.Name), ".")); name != "" {
				host.Hostnames = append(host.Hostnames, Hostname{Name: name, Kind: n.Type})
			}
		}

		for _, p := range h.Ports {
			host.Ports = append(host.Ports, Port{Number: p.ID, Protocol: strings.ToUpper(p.Protocol), State: p.State.State,
				Service: p.Service.Name, Product: p.Service.Product, Version: p.Service.Version})
		}

		report.Hosts = append(report.Hosts, host)
	}

	return report, nil
}
