package main

import (
	// "bytes"
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"

	// "text/template"
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

	// Get the main interface from /etc/main_interface
	mainInterface, err := getMainInterface()
	if err != nil {
		return nil, err
	}
	log.Println("main interface:", mainInterface)

	// Get all network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	// for _, iface := range ifaces {
	// 	log.Println("iface name:", iface.Name)
	// 	log.Println("equal iface:", (iface.Name == mainInterface))
	// 	log.Println("")
	// }

	// Iterate over the interfaces
	for _, iface := range ifaces {
		// Check if the interface name matches the main interface
		if iface.Name != mainInterface {
			continue
		}

		// Get all addresses for the interface
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		// Filter IPv6 addresses
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

	// Get the main interface from /etc/main_interface
	mainInterface, err := getMainInterface()
	if err != nil {
		return nil, err
	}

	// Get all network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	// Iterate over the interfaces
	for _, iface := range ifaces {
		// Check if the interface name matches the main interface
		if iface.Name != mainInterface {
			continue
		}

		// Get all addresses for the interface
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		// Filter IPv6 addresses
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
func applyTemplate(hostData []jinja2.Jinja2Opt) error {
	// Load the template from /etc/hosts_template.j2
	templateFile := "/etc/hosts_template.j2"
	templateBytes, err := os.ReadFile(templateFile)
	if err != nil {
		return fmt.Errorf("error reading template file: %w", err)
	}


	// Apply Jinja2 template
	// j, err := jinja2.NewJinja2("e", 1)
	j, err := jinja2.NewJinja2("", 1, hostData...)
	if err != nil {
		return fmt.Errorf("error applying jinja2 template: %w", err)
	}

	result, err := j.RenderString(string(templateBytes))
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
	test_comp()


	initialIPv6Addresses, err := getIPv6Addresses()
	if err != nil {
		log.Fatalf("Error getting IPv6 addresses: %v", err)
	}

	initialIPv4Addresses, err := getIPv4Addresses()
	if err != nil {
		log.Fatalf("Error getting IPv6 addresses: %v", err)
	}

	// Prepare the template content
	ipv6ListTemplate := ""


	ipv4ListTemplate := ""

	// hostData := HostData{
	// 	MainIPv4:         hostMainIPv4,
	// 	Hostname:         hostname,
	// 	HostnameExtra:    hostnameExtra,
	// 	IPv6ListTemplate: ipv6ListTemplate,
	// }

	var hostData []jinja2.Jinja2Opt
	var currentIPv6Addresses []string
	var currentIPv4Addresses []string

	var hostname string
	var hostnameExtra string


	// Monitor for changes in IPv6 addresses
	for {

		fmt.Println("getting v6 addrs")

		currentIPv6Addresses, err = getIPv6Addresses()
		if err != nil {
			log.Printf("Error getting IPv6 addresses: %v", err)
			continue
		}


		fmt.Println("getting v4 addrs")

		currentIPv4Addresses, err = getIPv4Addresses()
		if err != nil {
			log.Printf("Error getting IPv6 addresses: %v", err)
			continue
		}

		// Check if there are any changes
		if !equalIPv6Lists(initialIPv6Addresses, currentIPv6Addresses) || !equalIPv6Lists(initialIPv4Addresses, currentIPv4Addresses) {


			fmt.Println("getting hostname")

			hostname, hostnameExtra, err = getHostnameInfo()
			if err != nil {
				log.Fatalf("Error getting hostname information: %v", err)
			}
			log.Println("IPv6 addresses changed, updating /etc/hosts")

			// Update IPv6 list and reapply template
			ipv6ListTemplate = ""
			for _, ipv6 := range currentIPv6Addresses {
				ipv6ListTemplate += fmt.Sprintf("%s %s %s\n", ipv6, hostname, hostnameExtra)
			}

			ipv4ListTemplate = ""
			for _, ipv4 := range currentIPv4Addresses {
				ipv4ListTemplate += fmt.Sprintf("%s %s %s\n", ipv4, hostname, hostnameExtra)
			}


			hostData = []jinja2.Jinja2Opt{
				jinja2.WithGlobal("ipv6_host_replace", ipv6ListTemplate),
				jinja2.WithGlobal("ipv4_host_replace", ipv4ListTemplate),
				jinja2.WithGlobal("hostname_variable", hostname),
				jinja2.WithGlobal("hostname_variable_extra", hostnameExtra),
			}


			err = applyTemplate(hostData)
			if err != nil {
				log.Printf("Error applying template: %v", err)
				continue
			}

			// Update the initial list
			initialIPv6Addresses = currentIPv6Addresses
			initialIPv4Addresses = currentIPv4Addresses
		}

		time.Sleep(10 * time.Second) // Poll every 10 seconds
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

func test_comp() {
	t1 := []string{"test"}
	t2 := []string{"test"}
	if equalIPv6Lists(t1, t2) == false {
		log.Fatalln("list qual not working same list test")
	}

	t1 = []string{"test", "test3"}
	t2 = []string{"test", "test2"}
	if equalIPv6Lists(t1, t2) == true {
		log.Fatalln("list qual not working same list item diff items test")
	}
}
