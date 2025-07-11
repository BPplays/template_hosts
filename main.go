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
	"io"

	"github.com/Masterminds/sprig/v3"
	hostsfile "github.com/kevinburke/hostsfile/lib"
)

const (
	templateLocation = "/etc/hosts.tmpl"
)

// Struct to hold host data for templating
type HostData struct {
	IPv6HostReplace       string
	IPv4HostReplace       string
	HostnameVariable      string
}

func validateHosts(hosts string) (error) {
	var writer io.Writer = io.Discard

	hostsdec, err := hostsfile.Decode(strings.NewReader(hosts))
	if err != nil {
		return err
	}

	err = hostsfile.Encode(writer, hostsdec)
	if err != nil {
		return err
	}

	return nil

}

// Reads /etc/main_interfaces and returns a slice of interface names
func getMainInterfaces() ([]string, error) {
	file, err := os.Open("/etc/main_interfaces")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ifaces []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			ifaces = append(ifaces, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ifaces, nil
}

func getIPv6Addresses() ([]string, error) {
	var ipv6Addresses []string
	mainIfaces, err := getMainInterfaces()
	if err != nil {
		return nil, err
	}
	ifaceSet := make(map[string]bool)
	for _, name := range mainIfaces {
		ifaceSet[name] = true
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		if !ifaceSet[iface.Name] {
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
	mainIfaces, err := getMainInterfaces()
	if err != nil {
		return nil, err
	}
	ifaceSet := make(map[string]bool)
	for _, name := range mainIfaces {
		ifaceSet[name] = true
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		if !ifaceSet[iface.Name] {
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

func getHostnameInfo() (hostnames []string, err error) {
	hostname, err := os.Hostname()
	if err != nil {
		return []string{}, err
	}
	hostnames = append(hostnames, hostname)

	parts := strings.Split(hostname, ".")
	if len(parts) > 0 {
		hostnames = append(hostnames, parts[0])
	}
	return hostnames, nil
}

// applyTemplate uses Go text/template with Sprig funcs
func applyTemplate(data HostData) error {
	tmplBytes, err := os.ReadFile(templateLocation)
	if err != nil {
		return fmt.Errorf("error reading template file: %w", err)
	}

	tmpl, err := template.New("hosts").Funcs(sprig.FuncMap()).Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("error parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}

	result := fmt.Sprintf(
		"#\n#\n#\n# do not edit. this file was generated from %q\n#\n#\n#\n\n\n\n",
		templateLocation,
	) +
	buf.String()

	oldHosts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return fmt.Errorf("error reading /etc/hosts: %w", err)
	}

	err = validateHosts(result)
	if err != nil {
		log.Printf("validating new hosts file failed: %v", err)
		return err
	}

	err = os.WriteFile("/etc/hosts", []byte(result), 0644)

	if err != nil {
		var err2 error
		for range 10 {
			err2 = os.WriteFile("/etc/hosts", oldHosts, 0644)
			if err2 == nil { break }
			time.Sleep(1 * time.Second)
		}
		if err2 != nil {
			log.Println("!!! HOSTS FILE MAY BE IN BROKEN STATE, failed to restore old file")
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

	var prevV6, prevV4, prevHostnames []string

	for {
		v6Addrs, err := getIPv6Addresses()
		if err != nil {
			log.Printf("Error getting IPv6 addresses: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		v4Addrs, err := getIPv4Addresses()
		if err != nil {
			log.Printf("Error getting IPv4 addresses: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		hostnames, err := getHostnameInfo()
		if err != nil {
			log.Printf("Error getting hostname: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if len(hostnames) < 1 {
			log.Println("hostname empty string")
			time.Sleep(5 * time.Second)
			continue
		}

		if !equalLists(prevV6, v6Addrs) ||
		!equalLists(prevV4, v4Addrs) ||
		!equalLists(prevHostnames, hostnames) {
			var sb6, sb4 strings.Builder

			spaces := 0
			for _, ip := range v6Addrs {
				if len(ip) > spaces {
					spaces = len(ip)
				}
			}
			spaces += 4
			for _, ip := range v6Addrs {
				sb6.WriteString(fmt.Sprintf(
					"%s%s%s\n",
					ip,
					strings.Repeat(" ", spaces-len(ip)),
					strings.Join(hostnames, " "),
				))
			}


			spaces4 := 0
			for _, ip := range v4Addrs {
				if len(ip) > spaces {
					spaces4 = len(ip)
				}
			}
			for _, ip := range v4Addrs {
				sb4.WriteString(fmt.Sprintf(
					"%s%s%s\n",
					ip,
					strings.Repeat(" ", spaces4-len(ip)),
					strings.Join(hostnames, " "),
				))
			}

			log.Println(sb6.String())
			log.Println(sb4.String())

			data := HostData{
				IPv6HostReplace:       sb6.String(),
				IPv4HostReplace:       sb4.String(),
				HostnameVariable:      strings.Join(hostnames, " "),
			}

			err = applyTemplate(data)
			if err != nil {
				log.Printf("Error applying template: %v\n", err)
				continue
			}

			prevV6 = v6Addrs
			prevV4 = v4Addrs
		}

		log.Println("slept loop")
		time.Sleep(15 * time.Second)
	}
}
