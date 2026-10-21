// Package ingest implements `rex ingest`: running a scan tool, parsing its
// output, and folding the results into a report's artifacts/ and appendices/.
package ingest

import "encoding/xml"

// nmapRun is the subset of nmap's -oX schema needed to build an asset inventory.
type nmapRun struct {
	Hosts []nmapHost `xml:"host"`
}

type nmapHost struct {
	Status    nmapStatus    `xml:"status"`
	Addresses []nmapAddress `xml:"address"`
	Hostnames struct {
		Hostname []nmapHostname `xml:"hostname"`
	} `xml:"hostnames"`
	Ports struct {
		Port []nmapPort `xml:"port"`
	} `xml:"ports"`
	OS struct {
		OSMatch []nmapOSMatch `xml:"osmatch"`
	} `xml:"os"`
}

type nmapStatus struct {
	State string `xml:"state,attr"`
}

type nmapAddress struct {
	Addr     string `xml:"addr,attr"`
	AddrType string `xml:"addrtype,attr"`
}

type nmapHostname struct {
	Name string `xml:"name,attr"`
}

type nmapPort struct {
	Protocol string `xml:"protocol,attr"`
	PortID   string `xml:"portid,attr"`
	State    struct {
		State string `xml:"state,attr"`
	} `xml:"state"`
	Service struct {
		Name    string `xml:"name,attr"`
		Product string `xml:"product,attr"`
		Version string `xml:"version,attr"`
	} `xml:"service"`
}

type nmapOSMatch struct {
	Name string `xml:"name,attr"`
}

// parseNmapXML parses nmap -oX output into the tool-agnostic Asset shape,
// keeping only hosts that are up and ports that are open.
func parseNmapXML(data []byte) ([]Asset, error) {
	var run nmapRun
	if err := xml.Unmarshal(data, &run); err != nil {
		return nil, err
	}

	var assets []Asset
	for _, h := range run.Hosts {
		if h.Status.State != "up" {
			continue
		}

		asset := Asset{}
		for _, a := range h.Addresses {
			if a.AddrType == "ipv4" || a.AddrType == "ipv6" {
				asset.IP = a.Addr
				break
			}
		}
		if asset.IP == "" {
			continue
		}

		seenHostname := map[string]bool{}
		for _, hn := range h.Hostnames.Hostname {
			if hn.Name != "" && !seenHostname[hn.Name] {
				seenHostname[hn.Name] = true
				asset.Hostnames = append(asset.Hostnames, hn.Name)
			}
		}

		if len(h.OS.OSMatch) > 0 {
			asset.OS = h.OS.OSMatch[0].Name
		}

		for _, p := range h.Ports.Port {
			if p.State.State != "open" {
				continue
			}
			asset.Ports = append(asset.Ports, Port{
				Number:  p.PortID,
				Proto:   p.Protocol,
				Service: p.Service.Name,
				Product: p.Service.Product,
				Version: p.Service.Version,
			})
		}

		assets = append(assets, asset)
	}
	return assets, nil
}
