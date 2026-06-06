package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha3"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	hostsfile "github.com/kevinburke/hostsfile/lib"
	"github.com/vishvananda/netlink"
)

const (
	debug           = false
)

// Struct to hold host data for templating
type HostData struct {
	IPv6HostReplace       string
	IPv4HostReplace       string
	HostnameVariable      string

	IPv6IPs       []string
	IPv4IPs       []string
	Hostnames     []string
	Domains       []string

	OS string
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

func getTemplateDirLocation() (string) {
	switch strings.ToLower(runtime.GOOS) {
	case "linux":
		return "/etc/hosts_templates"

	case "windows":
		// return "C:\\ProgramData\\templates"
		return "C:\\Windows\\System32\\drivers\\etc\\hosts_templates"

	case "freebsd":
		return "/usr/local/etc/hosts_templates"

	default:
		return "/etc/hosts_templates"
	}
}

func getHostsLocation() (string) {
	switch strings.ToLower(runtime.GOOS) {
	case "linux":
		return "/etc/hosts"

	case "windows":
		return "C:\\Windows\\System32\\drivers\\etc\\hosts"

	case "freebsd":
		return "/etc/hosts"

	default:
		return "/etc/hosts"
	}
}

func indent(text string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)

	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}

	return strings.Join(lines, "\n")
}

func sortTemplateFiles(strs *[]string) () {
	slices.Sort(*strs)
}

func getTemplateFiles() ([]string, error) {
	dir := getTemplateDirLocation()
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".tmpl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error reading template dir %q: %w", dir, err)
	}

	sortTemplateFiles(&files)
	return files, nil
}

func hashFiles(files []string) ([]byte, error) {
	hash := sha3.New512()
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("error reading %q for hashing: %w", file, err)
		}
		hash.Write(data)
	}
	return hash.Sum(nil), nil
}

func applyTemplate(data HostData) error {
	tmplFiles, err := getTemplateFiles()
	if err != nil {
		return fmt.Errorf("error getting template files: %w", err)
	}

	var fullTmpl strings.Builder
	for _, file := range tmplFiles {
		tmplBytes, readErr := os.ReadFile(file)
		if readErr != nil {
			return fmt.Errorf("error reading template file %q: %w", file, readErr)
		}
		fullTmpl.WriteString("\n")
		fullTmpl.Write(tmplBytes)
	}

	tmpl, err := template.New("hosts").Funcs(sprig.FuncMap()).Parse(fullTmpl.String())
	if err != nil {
		return fmt.Errorf("error parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}

	result := fmt.Sprintf(
		"#\n#\n#\n# do not edit. this file was generated from %q\n#\n#\n#\n\n\n\n",
		getTemplateDirLocation(),
	) +
	buf.String()

	hostsFile := getHostsLocation()
	oldHosts, err := os.ReadFile(hostsFile)
	if err != nil {
		return fmt.Errorf("error reading hosts file %q: %w", hostsFile, err)
	}

	err = validateHosts(result)
	if err != nil {
		log.Printf("validating new hosts file failed: %v", err)
		fmt.Println(indent(result, 4))
		return err
	}

	err = os.WriteFile(hostsFile, []byte(result), 0644)

	if err != nil {
		var err2 error
		for range 10 {
			err2 = os.WriteFile(hostsFile, oldHosts, 0644)
			if err2 == nil { break }
			time.Sleep(1 * time.Second)
		}
		if err2 != nil {
			log.Println("!!! HOSTS FILE MAY BE IN BROKEN STATE, failed to restore old file")
		}
		return fmt.Errorf("error writing new hosts file %q: %w", hostsFile, err)
	}

	log.Println("wrote hosts file")
	return nil
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
		ifs, _ := net.Interfaces()
		for _, i := range ifs {
			fmt.Println("  ", i.Index, i.Name, i.HardwareAddr)
		}
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

		os := strings.ToLower(runtime.GOOS)

		var altNames []string
		if os != "windows" {
			altNames, err = getIfAltnames(iface.Name)
			if err != nil {
				log.Printf("can't get altnames: %v", err)
			}

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

func joinHostnameDomain(hostname, domain string) string {
	hostname = strings.TrimSuffix(hostname, ".")
	domain = strings.TrimPrefix(domain, ".")

	if hostname == "" {
		return domain
	}
	if domain == "" {
		return hostname
	}

	return hostname + "." + domain
}

func addDomainToHostnames(hostnames []string, domains []string) ([]string) {
	var output []string
	output = append(output, hostnames...)

	for _, hostname := range hostnames {
		for _, domain := range domains {
			output = append(output, joinHostnameDomain(hostname, domain))
		}
	}
	return output
}

// splits a hostname by . and outputs a slice of combined ones except the original
func getHostnameSplits(s string) (hostnames []string) {
	parts := strings.Split(s, ".")

	for i := range len(parts)-1 {
		tmp := []string{}
		for i2 := range i+1 {
			tmp = append(tmp, parts[i2])
		}
		hostnames = append(hostnames, strings.Join(tmp, "."))
	}
	return
}

func getHostnameInfo() (hostnames []string, err error) {

	hostname, err := os.Hostname()
	if err != nil {
		return []string{"hostnamefallback.fallbackfakedomain"}, err
	}
	hostnames = append(hostnames, hostname)

	hostnames = append(hostnames, getHostnameSplits(hostname)...)

	if strings.ToLower(runtime.GOOS) == "windows" {
		domains, err := getTCPIPDomain()
		if err != nil {
			fmt.Println("can't get domains")
		}
		if len(domains) > 0 {
			hostnames = addDomainToHostnames(hostnames, domains)
		}
	}

	return hostnames, nil
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

func start(ctx context.Context) {
	log.SetFlags(0)


	var prevV6, prevV4, prevHostnames []string
	var prevTmplHash []byte

	for {
		if ctx.Err() != nil {
			return
		}

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

		tmplFilesForHash, hashErr := getTemplateFiles()
		if hashErr != nil {
			log.Printf("Can't get template files, error: %v\n", hashErr)
			time.Sleep(5 * time.Second)
			continue
		}
		tmplHash, err := hashFiles(tmplFilesForHash)
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
					strings.Repeat(" ", max(spaces-len(ip), 1)),
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
					strings.Repeat(" ", max(spaces4-len(ip), 1)),
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

				IPv6IPs: v6Addrs,
				IPv4IPs: v4Addrs,
				Hostnames:      hostnames,

				OS: strings.ToLower(runtime.GOOS),
			}

			err = applyTemplate(data)
			if err != nil {
				log.Printf("Error applying template: %v\n", err)
				time.Sleep(10 * time.Second)
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

func main() {

	srvAction := flag.String("service", "", "install|uninstall|start|stop|run")
	flag.Parse()

	if *srvAction != "" {
		err := serviceAction(srvAction)
		if err != nil {
			fmt.Println(err)
		}

		return
	}

	ctx := context.Background()
	if strings.ToLower(runtime.GOOS) == "windows" {
		err := srvMain()
		if err != nil {
			fmt.Println(err)
			return
		}

	} else {
		start(ctx)
	}
}
