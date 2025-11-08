package main

import (
	"bufio"
	"bytes"
	"crypto/sha3"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	hostsfile "github.com/kevinburke/hostsfile/lib"
	"github.com/vishvananda/netlink"
)

const (
	debug = false
)

// Struct to hold host data for templating
type HostData struct {
	IPv6HostReplace       string
	IPv4HostReplace       string
	HostnameVariable      string
}

func hashFile(path string) ([]byte, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	hash, err := defHash(&file)

	return *hash, nil
}

func defHash(input *[]byte) (*[]byte, error) {

	hash := sha3.New512()
	_, err := hash.Write(*input)
	if err != nil {
		return nil, err
	}

	sum := hash.Sum(nil)

	return &sum, nil
}

func getTemplateLocation() (string) {
	switch strings.ToLower(runtime.GOOS) {
	case "linux":
		return "/etc/hosts.tmpl"

	case "windows":
		return "C:\\Windows\\System32\\drivers\\etc\\hosts.tmpl"

	case "freebsd":
		return "/usr/local/etc/hosts.tmpl"

	default:
		return "/etc/hosts.tmpl"
	}
}

func getMainIfLocation() (string) {
	switch strings.ToLower(runtime.GOOS) {
	case "linux":
		return "/etc/main_interfaces"

	case "windows":
		return "C:\\ProgramData\\main_interfaces"

	case "freebsd":
		return "/usr/local/etc/main_interfaces"

	default:
		return "/etc/main_interfaces"
	}
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


	file, err := os.Open(getMainIfLocation())
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

func getIfAltnames(iface string) ([]string, error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return []string{}, fmt.Errorf("can't get iface: %w", err)
	}

	return link.Attrs().AltNames, nil
}

func tryIPToNetip(ip net.IP) (addr netip.Addr, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	ok = true

	addr, ok = netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}

	if addr.Is4In6() {
		addr = netip.AddrFrom4(addr.As4())
	}

	return addr, ok
}

func isIPv6(ipNet *netip.Addr) (bool) {
	switch {
	case ipNet.Is4():
		return false
	case !ipNet.Is6():
		return false
	case ipNet.IsLoopback():
		return false
	case ipNet.IsLinkLocalUnicast():
		return false
	case ipNet.IsLinkLocalMulticast():
		return false
	case ipNet.IsUnspecified():
		return false
	case !ipNet.IsValid():
		return false
	}
	return true
}

func isIPv4(ipNet *netip.Addr) (bool) {
	switch {
	case !ipNet.Is4():
		return false
	case ipNet.Is6():
		return false
	case ipNet.IsLoopback():
		return false
	case ipNet.IsLinkLocalUnicast():
		return false
	case ipNet.IsLinkLocalMulticast():
		return false
	case ipNet.IsUnspecified():
		return false
	case !ipNet.IsValid():
		return false
	}
	return true
}

func getIPaddresses(validateFunc func(*netip.Addr) bool) ([]string, error) {
	var ipAddresses []string
	mainIfaces, err := getMainInterfaces()
	if err != nil {
		return nil, err
	}
	log.Printf("mainifaces: %v", mainIfaces)

	ifaceSet := make(map[string]bool)
	for _, name := range mainIfaces {
		ifaceSet[name] = true
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		ifaceNames := []string{iface.Name}
		ifaceFound := false

		altNames, err := getIfAltnames(iface.Name)
		if err != nil {
			log.Printf("can't get altnames: %v", err)
		}

		ifaceNames = append(ifaceNames, altNames...)

		for _, ifaceName := range ifaceNames {
			if ifaceSet[ifaceName] {
				ifaceFound = true
				break
			}
		}
		if !ifaceFound {
			if debug {
				log.Printf("iface skipped: %v", iface.Name)
			}
			continue
		}


		addrs, err := iface.Addrs()
		if debug {
			log.Printf("iface new addr: %v", addrs)
		}
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {

			ipNet, ok := addr.(*net.IPNet)
			if !ok { continue }
			prefix, ok := tryIPToNetip((*ipNet).IP)
			if !ok { continue }
			if !validateFunc(&prefix) {
				continue
			}
			ipAddresses = append(ipAddresses, ipNet.IP.String())
		}
	}

	return ipAddresses, nil
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
	tmplBytes, err := os.ReadFile(getTemplateLocation())
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
		getTemplateLocation(),
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

func equalStrLists(a, b []string) bool {
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
	var prevTmplHash []byte

	for {
		start := time.Now()
		v6Addrs, err := getIPaddresses(isIPv6)
		if err != nil {
			log.Printf("Error getting IPv6 addresses: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		v4Addrs, err := getIPaddresses(isIPv4)
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

		tmplHash, err := hashFile(getTemplateLocation())
		if err != nil {
			log.Printf("Can't read hash, error: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if !equalStrLists(prevV6, v6Addrs) ||
		!equalStrLists(prevV4, v4Addrs) ||
		!equalStrLists(prevHostnames, hostnames) ||
		!bytes.Equal(tmplHash, prevTmplHash) {
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

			log.Println("6 addrs:", v6Addrs)
			log.Println("sb6:", sb6.String())
			log.Println("sb4:", sb4.String())

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
			prevHostnames = hostnames
			prevTmplHash = tmplHash
		}


		fmt.Printf("loop time taken: %s\n", time.Since(start))
		time.Sleep(15 * time.Second)
	}
}
