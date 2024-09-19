package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"text/template"
	"time"

	jinja2 "github.com/kluctl/go-jinja2"
)

// Struct to hold host data for templating
type HostData struct {
	MainIPv4          string
	Hostname          string
	HostnameExtra     string
	IPv6ListTemplate  string
}

// Function to get all IPv6 addresses of the system
func getIPv6Addresses() ([]string, error) {
	var ipv6Addresses []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() != nil {
				continue
			}
			if ipNet.IP.To16() != nil {
				ipv6Addresses = append(ipv6Addresses, ipNet.IP.String())
			}
		}
	}
	return ipv6Addresses, nil
}

// Function to get the main IPv4 address
func getMainIPv4() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if ok && ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("No main IPv4 address found")
}

// Function to get the hostname and the first part of the hostname
func getHostnameInfo() (string, string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", "", err
	}

	hostnameParts := strings.Split(hostname, ".")
	if len(hostnameParts) > 0 {
		return hostname, hostnameParts[0], nil
	}

	return hostname, hostname, nil
}

// Function to apply the Jinja2 template and write to /etc/hosts
func applyTemplate(hostData HostData) error {
	// Load the template from /etc/hosts_template.j2
	templateFile := "/etc/hosts_template.j2"
	templateBytes, err := ioutil.ReadFile(templateFile)
	if err != nil {
		return fmt.Errorf("error reading template file: %w", err)
	}

	// Apply Jinja2 template
	j := jinja2.NewJinja2()
	result, err := j.RenderString(string(templateBytes), hostData)
	if err != nil {
		return fmt.Errorf("error applying jinja2 template: %w", err)
	}

	// Write the result to /etc/hosts
	err = ioutil.WriteFile("/etc/hosts", []byte(result), 0644)
	if err != nil {
		return fmt.Errorf("error writing to /etc/hosts: %w", err)
	}

	return nil
}

// Main function to monitor and apply changes
func main() {
	// Fetch initial host data
	hostMainIPv4, err := getMainIPv4()
	if err != nil {
		log.Fatalf("Error getting main IPv4 address: %v", err)
	}

	hostname, hostnameExtra, err := getHostnameInfo()
	if err != nil {
		log.Fatalf("Error getting hostname information: %v", err)
	}

	// Initial list of IPv6 addresses
	initialIPv6Addresses, err := getIPv6Addresses()
	if err != nil {
		log.Fatalf("Error getting IPv6 addresses: %v", err)
	}

	// Prepare the template content
	ipv6ListTemplate := ""
	for _, ipv6 := range initialIPv6Addresses {
		ipv6ListTemplate += fmt.Sprintf("%s %s %s\n", ipv6, hostname, hostnameExtra)
	}

	hostData := HostData{
		MainIPv4:         hostMainIPv4,
		Hostname:         hostname,
		HostnameExtra:    hostnameExtra,
		IPv6ListTemplate: ipv6ListTemplate,
	}

	// Apply the template initially
	err = applyTemplate(hostData)
	if err != nil {
		log.Fatalf("Error applying template: %v", err)
	}

	// Monitor for changes in IPv6 addresses
	for {
		time.Sleep(10 * time.Second) // Poll every 10 seconds

		// Get the current list of IPv6 addresses
		currentIPv6Addresses, err := getIPv6Addresses()
		if err != nil {
			log.Printf("Error getting IPv6 addresses: %v", err)
			continue
		}

		// Check if there are any changes
		if !equalIPv6Lists(initialIPv6Addresses, currentIPv6Addresses) {
			log.Println("IPv6 addresses changed, updating /etc/hosts")

			// Update IPv6 list and reapply template
			ipv6ListTemplate = ""
			for _, ipv6 := range currentIPv6Addresses {
				ipv6ListTemplate += fmt.Sprintf("%s %s %s\n", ipv6, hostname, hostnameExtra)
			}

			hostData.IPv6ListTemplate = ipv6ListTemplate

			err = applyTemplate(hostData)
			if err != nil {
				log.Printf("Error applying template: %v", err)
				continue
			}

			// Update the initial list
			initialIPv6Addresses = currentIPv6Addresses
		}
	}
}

// Function to compare two lists of IPv6 addresses
func equalIPv6Lists(list1, list2 []string) bool {
	if len(list1) != len(list2) {
		return false
	}

	addrMap := make(map[string]bool)
	for _, addr := range list1 {
		addrMap[addr] = true
	}

	for _, addr := range list2 {
		if !addrMap[addr] {
			return false
		}
	}

	return true
}
