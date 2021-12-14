/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	utiljson "encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apiserver/pkg/util/logs"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	"k8s.io/kubernetes/pkg/version/verflag"
	utilexec "k8s.io/utils/exec"

	"github.com/Azure/ip-masq-agent-v2/cmd/ip-masq-agent/testing/fakefs"
	"github.com/golang/glog"
)

const (
	linkLocalCIDR = "169.254.0.0/16"
	// RFC 4291
	linkLocalCIDRIPv6 = "fe80::/10"
	// TODO change the below to configurable flags
	// path to a yaml or json files
	configPath = "/etc/config/"
	// config files in this path must start with this to be read
	configFilePrefix = "ip-masq"
)

var (
	// name of nat chain for iptables masquerade rules
	masqChain                         utiliptables.Chain
	masqChainFlag                     = flag.String("masq-chain", "IP-MASQ-AGENT", `Name of nat chain for iptables masquerade rules.`)
	noMasqueradeAllReservedRangesFlag = flag.Bool("nomasq-all-reserved-ranges", false, "Whether to disable masquerade for all IPv4 ranges reserved by RFCs.")
	enableIPv6                        = flag.Bool("enable-ipv6", false, "Whether to enable IPv6.")
	resyncInterval                    = flag.Int("resync-interval", 60, "How often to refresh the config (in seconds)")
)

// MasqConfig object
type MasqConfig struct {
	NonMasqueradeCIDRs []string `json:"nonMasqueradeCIDRs"`
	MasqLinkLocal      bool     `json:"masqLinkLocal"`
	MasqLinkLocalIPv6  bool     `json:"masqLinkLocalIPv6"`
}

// Duration - Go's JSON unmarshaler can't handle time.ParseDuration syntax when unmarshaling into time.Duration, so we do it here
type Duration time.Duration

// UnmarshalJSON ...
func (d *Duration) UnmarshalJSON(json []byte) error {
	if json[0] == '"' {
		s := string(json[1 : len(json)-1])
		t, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		*d = Duration(t)
		return nil
	}
	s := string(json)
	return fmt.Errorf("expected string value for unmarshal to field of type Duration, got %q", s)
}

// EmptyMasqConfig returns a MasqConfig with empty values
func EmptyMasqConfig() *MasqConfig {
	return &MasqConfig{
		NonMasqueradeCIDRs: make([]string, 0),
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  false,
	}
}

// DefaultMasqConfig returns a MasqConfig with default values, intended to be used when no config is found
func DefaultMasqConfig(masqAllReservedRanges bool) *MasqConfig {
	// RFC 1918 defines the private ip address space as 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	nonMasq := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

	if masqAllReservedRanges {
		nonMasq = append(nonMasq,
			"100.64.0.0/10",   // RFC 6598
			"192.0.0.0/24",    // RFC 6890
			"192.0.2.0/24",    // RFC 5737
			"192.88.99.0/24",  // RFC 7526
			"198.18.0.0/15",   // RFC 6815
			"198.51.100.0/24", // RFC 5737
			"203.0.113.0/24",  // RFC 5737
			"240.0.0.0/4")     // RFC 5735, Former Class E range obsoleted by RFC 3232
	}

	return &MasqConfig{
		NonMasqueradeCIDRs: nonMasq,
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  false,
	}
}

// MasqDaemon object
type MasqDaemon struct {
	config    *MasqConfig
	iptables  utiliptables.Interface
	ip6tables utiliptables.Interface
}

// NewMasqDaemon returns a MasqDaemon with default values, including an initialized utiliptables.Interface
func NewMasqDaemon(c *MasqConfig) *MasqDaemon {
	execer := utilexec.New()
	dbus := utildbus.New()
	protocolv4 := utiliptables.ProtocolIpv4
	protocolv6 := utiliptables.ProtocolIpv6
	iptables := utiliptables.New(execer, dbus, protocolv4)
	ip6tables := utiliptables.New(execer, dbus, protocolv6)
	return &MasqDaemon{
		config:    c,
		iptables:  iptables,
		ip6tables: ip6tables,
	}
}

func main() {
	flag.Parse()
	masqChain = utiliptables.Chain(*masqChainFlag)

	c := DefaultMasqConfig(*noMasqueradeAllReservedRangesFlag)

	logs.InitLogs()
	defer logs.FlushLogs()

	verflag.PrintAndExitIfRequested()

	m := NewMasqDaemon(c)
	m.Run()
}

// Run ...
func (m *MasqDaemon) Run() {
	// Periodically resync to reconfigure or heal from any rule decay
	for {
		func() {
			defer time.Sleep(time.Duration(*resyncInterval) * time.Second)
			// resync config
			if err := m.osSyncConfig(); err != nil {
				glog.Errorf("error syncing configuration: %v", err)
				return
			}
			// resync rules
			if err := m.syncMasqRules(); err != nil {
				glog.Errorf("error syncing masquerade rules: %v", err)
				return
			}
			// resync ipv6 rules
			if err := m.syncMasqRulesIPv6(); err != nil {
				glog.Errorf("error syncing masquerade rules for ipv6: %v", err)
				return
			}
		}()
	}
}

func (m *MasqDaemon) osSyncConfig() error {
	// the fakefs.FileSystem interface allows us to mock the fs from tests
	// fakefs.DefaultFS implements fakefs.FileSystem using os.Stat and io/ioutil.ReadFile
	var fs fakefs.FileSystem = fakefs.DefaultFS{}
	return m.syncConfig(fs)
}

// Syncs the config to the file at ConfigPath, or uses defaults if the file could not be found
// Error if the file is found but cannot be parsed.
func (m *MasqDaemon) syncConfig(fs fakefs.FileSystem) error {
	var err error
	c := EmptyMasqConfig()
	defer func() {
		if err == nil {
			json, _ := utiljson.Marshal(c)
			glog.V(2).Infof("using config: %s", string(json))
		}
	}()

	files, err := os.ReadDir(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config directory, error: %w", err)
	}

	var configAdded bool
	for _, file := range files {
		if strings.HasPrefix(file.Name(), configFilePrefix) {
			glog.V(2).Infof("found config file %s at %q", file.Name(), configPath)
			yaml, err := fs.ReadFile(filepath.Join(configPath, file.Name()))
			if err != nil {
				return fmt.Errorf("failed to read config file %s, error: %w", file.Name(), err)
			}

			json, err := utilyaml.ToJSON(yaml)
			if err != nil {
				return fmt.Errorf("failed to convert config file %s to JSON, error: %w", file.Name(), err)
			}

			var newConfig MasqConfig
			if err = utiljson.Unmarshal(json, &newConfig); err != nil {
				return fmt.Errorf("failed to unmarshal config file %s, error: %w", file.Name(), err)
			}

			c.merge(&newConfig)

			configAdded = true
		}
	}

	if !configAdded {
		// no valid config files found, use defaults
		c = DefaultMasqConfig(*noMasqueradeAllReservedRangesFlag)
		glog.V(2).Infof("no valid config files found at %q, using default values", configPath)
	}

	// validate configuration
	if err := c.validate(); err != nil {
		return fmt.Errorf("config is invalid, error: %w", err)
	}

	// apply new config
	m.config = c

	return nil
}

func (c *MasqConfig) validate() error {
	// limit to 64 CIDRs (excluding link-local) to protect against really bad mistakes
	n := len(c.NonMasqueradeCIDRs)
	if n > 64 {
		return fmt.Errorf("the daemon can only accept up to 64 CIDRs (excluding link-local), but got %d CIDRs (excluding link local)", n)
	}
	// check CIDRs are valid
	for _, cidr := range c.NonMasqueradeCIDRs {
		if err := validateCIDR(cidr); err != nil {
			return err
		}
		// can't configure ipv6 cidr if ipv6 is not enabled
		if !*enableIPv6 && isIPv6CIDR(cidr) {
			return fmt.Errorf("ipv6 is not enabled, but ipv6 cidr %s provided. Enable ipv6 using --enable-ipv6 agent flag", cidr)
		}
	}
	return nil
}

// merge combines the existing MasqConfig with newConfig. The bools are OR'd together.
func (c *MasqConfig) merge(newConfig *MasqConfig) {
	if newConfig.NonMasqueradeCIDRs != nil && len(newConfig.NonMasqueradeCIDRs) > 0 {
		c.NonMasqueradeCIDRs = mergeCIDRs(c.NonMasqueradeCIDRs, newConfig.NonMasqueradeCIDRs)
	}

	c.MasqLinkLocal = c.MasqLinkLocal || newConfig.MasqLinkLocal
	c.MasqLinkLocalIPv6 = c.MasqLinkLocalIPv6 || newConfig.MasqLinkLocalIPv6
}

// mergeCIDRS merges two slices of CIDRs into one, ignoring duplicates
func mergeCIDRs(cidrs1, cidrs2 []string) []string {
	cidrsSet := map[string]struct{}{}

	for _, cidr := range cidrs1 {
		cidrsSet[cidr] = struct{}{}
	}

	for _, cidr := range cidrs2 {
		cidrsSet[cidr] = struct{}{}
	}

	var cidrsList []string
	for cidr := range cidrsSet {
		cidrsList = append(cidrsList, cidr)
	}

	return cidrsList
}

func min(x, y Duration) Duration {
	if x < y {
		return x
	}
	return y
}

const cidrParseErrFmt = "CIDR %q could not be parsed, %v"
const cidrAlignErrFmt = "CIDR %q is not aligned to a CIDR block, ip: %q network: %q"

func validateCIDR(cidr string) error {
	// parse test
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf(cidrParseErrFmt, cidr, err)
	}
	// alignment test
	if !ip.Equal(ipnet.IP) {
		return fmt.Errorf(cidrAlignErrFmt, cidr, ip, ipnet.String())
	}
	return nil
}

func (m *MasqDaemon) syncMasqRules() error {
	// make sure our custom chain for non-masquerade exists
	m.iptables.EnsureChain(utiliptables.TableNAT, masqChain)

	// ensure that any non-local in POSTROUTING jumps to masqChain
	if err := m.ensurePostroutingJump(); err != nil {
		return err
	}

	// build up lines to pass to iptables-restore
	lines := bytes.NewBuffer(nil)
	writeLine(lines, "*nat")
	writeLine(lines, utiliptables.MakeChainLine(masqChain)) // effectively flushes masqChain atomically with rule restore

	// link-local CIDR is always non-masquerade
	if !m.config.MasqLinkLocal {
		writeNonMasqRule(lines, linkLocalCIDR)
	}

	// non-masquerade for user-provided CIDRs
	for _, cidr := range m.config.NonMasqueradeCIDRs {
		if !isIPv6CIDR(cidr) {
			writeNonMasqRule(lines, cidr)
		}
	}

	// masquerade all other traffic that is not bound for a --dst-type LOCAL destination
	writeMasqRule(lines)

	writeLine(lines, "COMMIT")

	if err := m.iptables.RestoreAll(lines.Bytes(), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters); err != nil {
		return err
	}
	return nil
}

func (m *MasqDaemon) syncMasqRulesIPv6() error {
	isIPv6Enabled := *enableIPv6

	if isIPv6Enabled {
		// make sure our custom chain for ipv6 non-masquerade exists
		_, err := m.ip6tables.EnsureChain(utiliptables.TableNAT, masqChain)
		if err != nil {
			return err
		}
		// ensure that any non-local in POSTROUTING jumps to masqChain
		if err := m.ensurePostroutingJumpIPv6(); err != nil {
			return err
		}
		// build up lines to pass to ip6tables-restore
		lines6 := bytes.NewBuffer(nil)
		writeLine(lines6, "*nat")
		writeLine(lines6, utiliptables.MakeChainLine(masqChain)) // effectively flushes masqChain atomically with rule restore

		// link-local IPv6 CIDR is non-masquerade by default
		if !m.config.MasqLinkLocalIPv6 {
			writeNonMasqRule(lines6, linkLocalCIDRIPv6)
		}

		for _, cidr := range m.config.NonMasqueradeCIDRs {
			if isIPv6CIDR(cidr) {
				writeNonMasqRule(lines6, cidr)
			}
		}

		// masquerade all other traffic that is not bound for a --dst-type LOCAL destination
		writeMasqRule(lines6)

		writeLine(lines6, "COMMIT")

		if err := m.ip6tables.RestoreAll(lines6.Bytes(), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters); err != nil {
			return err
		}
	}
	return nil
}

// NOTE(mtaufen): iptables requires names to be <= 28 characters, and somehow prepending "-m comment --comment " to this string makes it think this condition is violated
// Feel free to dig around in iptables and see if you can figure out exactly why; I haven't had time to fully trace how it parses and handle subcommands.
// If you want to investigate, get the source via `git clone git://git.netfilter.org/iptables.git`, `git checkout v1.4.21` (the version I've seen this issue on,
// though it may also happen on others), and start with `git grep XT_EXTENSION_MAXNAMELEN`.
func postroutingJumpComment() string {
	return fmt.Sprintf("ip-masq-agent: ensure nat POSTROUTING directs all non-LOCAL destination traffic to our custom %s chain", masqChain)
}

func (m *MasqDaemon) ensurePostroutingJump() error {
	if _, err := m.iptables.EnsureRule(utiliptables.Append, utiliptables.TableNAT, utiliptables.ChainPostrouting,
		"-m", "comment", "--comment", postroutingJumpComment(),
		"-m", "addrtype", "!", "--dst-type", "LOCAL", "-j", string(masqChain)); err != nil {
		return fmt.Errorf("failed to ensure that %s chain %s jumps to MASQUERADE: %v", utiliptables.TableNAT, masqChain, err)
	}
	return nil
}

func (m *MasqDaemon) ensurePostroutingJumpIPv6() error {
	if _, err := m.ip6tables.EnsureRule(utiliptables.Append, utiliptables.TableNAT, utiliptables.ChainPostrouting,
		"-m", "comment", "--comment", postroutingJumpComment(),
		"-m", "addrtype", "!", "--dst-type", "LOCAL", "-j", string(masqChain)); err != nil {
		return fmt.Errorf("failed to ensure that %s chain %s jumps to MASQUERADE: %v for ipv6", utiliptables.TableNAT, masqChain, err)
	}
	return nil
}

const nonMasqRuleComment = `-m comment --comment "ip-masq-agent: local traffic is not subject to MASQUERADE"`

func writeNonMasqRule(lines *bytes.Buffer, cidr string) {
	writeRule(lines, utiliptables.Append, masqChain, nonMasqRuleComment, "-d", cidr, "-j", "RETURN")
}

const masqRuleComment = `-m comment --comment "ip-masq-agent: outbound traffic is subject to MASQUERADE (must be last in chain)"`

func writeMasqRule(lines *bytes.Buffer) {
	writeRule(lines, utiliptables.Append, masqChain, masqRuleComment, "-j", "MASQUERADE")
}

// Similar syntax to utiliptables.Interface.EnsureRule, except you don't pass a table
// (you must write these rules under the line with the table name)
func writeRule(lines *bytes.Buffer, position utiliptables.RulePosition, chain utiliptables.Chain, args ...string) {
	fullArgs := append([]string{string(position), string(chain)}, args...)
	writeLine(lines, fullArgs...)
}

// Join all words with spaces, terminate with newline and write to buf.
func writeLine(lines *bytes.Buffer, words ...string) {
	lines.WriteString(strings.Join(words, " ") + "\n")
}

// isIPv6CIDR checks if the provided cidr block belongs to ipv6 family.
// If cidr belongs to ipv6 family, return true else it returns false
// which means the cidr belongs to ipv4 family
func isIPv6CIDR(cidr string) bool {
	ip, _, _ := net.ParseCIDR(cidr)
	return isIPv6(ip.String())
}

// isIPv6 checks if the provided ip belongs to ipv6 family.
// If ip belongs to ipv6 family, return true else it returns false
// which means the ip belongs to ipv4 family
func isIPv6(ip string) bool {
	return net.ParseIP(ip).To4() == nil
}
