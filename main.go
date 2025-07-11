package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
)

// Struct to hold host data for templating
// Fields match template variables
type HostData struct {
	IPv6HostReplace       string
	IPv4HostReplace       string
	HostnameVariable      string
	HostnameVariableExtra string
}

func getMainInterface() (string, error) {
	file, err := os.Open("/etc/main_interface")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	return "", fmt.Errorf("could not read main interface")
}

func getIPv6Addresses() ([]string, error) {
	var ipv6Addresses []string
	mainInterface, err := getMainInterface()
	if err != nil {
		return nil, err
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		if iface.Name != mainInterface {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() != nil || ipNet.IP.To16() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ipv6Addresses = append(ipv6Addresses, ipNet.IP.String())
		}
	}

	return ipv6Addresses, nil
}

func getIPv4Addresses() ([]string, error) {
	var ipv4Addresses []string
	mainInterface, err := getMainInterface()
	if err != nil {
		return nil, err
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		if iface.Name != mainInterface {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ipv4Addresses = append(ipv4Addresses, ipNet.IP.String())
		}
	}

	return ipv4Addresses, nil
}

func getHostnameInfo() (string, string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(hostname, ".")
	if len(parts) > 0 {
		return hostname, parts[0], nil
	}
	return hostname, hostname, nil
}

// applyTemplate uses Go text/template with Sprig funcs
func applyTemplate(data HostData) error {
	// read template file
	tmplBytes, err := os.ReadFile("/etc/hosts_template.j2")
	if err != nil {
		return fmt.Errorf("error reading template file: %w", err)
	}

	// create and parse template
	tmpl, err := template.New("hosts").Funcs(sprig.FuncMap()).Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("error parsing template: %w", err)
	}

	// execute into buffer
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}

	result := "#\n#\n#\n# do not edit. this file was generated from \"/etc/hosts_template.j2\"\n#\n#\n#\n\n\n\n" + buf.String()

	// backup old hosts
	oldHosts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return fmt.Errorf("error reading /etc/hosts: %w", err)
	}

	// write new hosts
	if err := os.WriteFile("/etc/hosts", []byte(result), 0644); err != nil {
		// restore backup
		err2 := os.WriteFile("/etc/hosts", oldHosts, 0644)
		if err2 != nil {
			log.Fatalln("!!! HOSTS FILE MAY BE IN BROKEN STATE, failed to restore old file")
		}
		return fmt.Errorf("error writing new /etc/hosts: %w", err)
	}

	log.Println("wrote hosts file")
	return nil
}

func equalLists(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]bool)
	for _, v := range a {
		m[v] = true
	}
	for _, v := range b {
		if !m[v] {
			return false
		}
	}
	return true
}

func main() {
	log.SetFlags(0)

	var initialV6, initialV4 []string

	for {
		v6Addrs, err := getIPv6Addresses()
		if err != nil {
			log.Printf("Error getting IPv6 addresses: %v", err)
			continue
		}

		v4Addrs, err := getIPv4Addresses()
		if err != nil {
			log.Printf("Error getting IPv4 addresses: %v", err)
			continue
		}

		if !equalLists(initialV6, v6Addrs) || !equalLists(initialV4, v4Addrs) {
			hostname, hostnameExtra, err := getHostnameInfo()
			if err != nil {
				log.Fatalf("Error getting hostname: %v", err)
			}

			// build list templates
			var sb6, sb4 strings.Builder
			spaces := 0
			for _, ip := range v6Addrs {
				if len(ip) > spaces {
					spaces = len(ip)
				}
			}
			spaces += 4
			for _, ip := range v6Addrs {
				sb6.WriteString(fmt.Sprintf("%s%s%s %s\n", ip, strings.Repeat(" ", spaces-len(ip)), hostname, hostnameExtra))
			}
			for _, ip := range v4Addrs {
				sb4.WriteString(fmt.Sprintf("%s    %s %s\n", ip, hostname, hostnameExtra))
			}

			data := HostData{
				IPv6HostReplace:       sb6.String(),
				IPv4HostReplace:       sb4.String(),
				HostnameVariable:      hostname,
				HostnameVariableExtra: hostnameExtra,
			}

			err = applyTemplate(data)
			if err != nil {
				log.Printf("Error applying template: %v", err)
				continue
			}

			initialV6 = v6Addrs
			initialV4 = v4Addrs
		}

		log.Println("slept loop")
		time.Sleep(150 * time.Second)
	}
}
