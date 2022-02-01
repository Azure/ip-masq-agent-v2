/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or impliem.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	iptest "k8s.io/kubernetes/pkg/util/iptables/testing"

	"github.com/Azure/ip-masq-agent-v2/cmd/ip-masq-agent-v2/testing/fakefs"
)

// turn off glog logging during tests to avoid clutter in output
func TestMain(m *testing.M) {
	_ = flag.Set("logtostderr", "false")
	ec := m.Run()
	os.Exit(ec)
}

// returns a MasqDaemon with empty config values and a fake iptables interface
func NewFakeMasqDaemon() *MasqDaemon {
	return &MasqDaemon{
		config:    &MasqConfig{},
		iptables:  iptest.NewFake(),
		ip6tables: iptest.NewFake(),
	}
}

// Returns a MasqConfig with config values that are the same as the default values when the
// noMasqueradeAllReservedRangesFlag is false.
func NewMasqConfigNoReservedRanges() *MasqConfig {
	return &MasqConfig{
		NonMasqueradeCIDRs: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
		MasqLinkLocal:      false,
	}
}

// Returns a MasqConfig with config values that are the same as the default values when the
// noMasqueradeAllReservedRangesFlag is true.
func NewMasqConfigWithReservedRanges() *MasqConfig {
	return &MasqConfig{
		NonMasqueradeCIDRs: []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"100.64.0.0/10",
			"192.0.0.0/24",
			"192.0.2.0/24",
			"192.88.99.0/24",
			"198.18.0.0/15",
			"198.51.100.0/24",
			"203.0.113.0/24",
			"240.0.0.0/4"},
		MasqLinkLocal: false,
	}
}

// specs for testing config validation
var validateConfigTests = []struct {
	cfg *MasqConfig
	err error
}{
	// Empty CIDR List
	{&MasqConfig{}, nil},
	// Default Config
	{NewMasqConfigNoReservedRanges(), nil},
	// CIDR that doesn't match regex
	{&MasqConfig{NonMasqueradeCIDRs: []string{"abcdefg"}}, fmt.Errorf(cidrParseErrFmt, "abcdefg", fmt.Errorf("invalid CIDR address: %s", "abcdefg"))},
	// Multiple CIDRs, one doesn't match regex
	{&MasqConfig{NonMasqueradeCIDRs: []string{"10.0.0.0/8", "abcdefg"}}, fmt.Errorf(cidrParseErrFmt, "abcdefg", fmt.Errorf("invalid CIDR address: %s", "abcdefg"))},
	// CIDR that matches regex but can't be parsed
	{&MasqConfig{NonMasqueradeCIDRs: []string{"10.256.0.0/16"}}, fmt.Errorf(cidrParseErrFmt, "10.256.0.0/16", fmt.Errorf("invalid CIDR address: %s", "10.256.0.0/16"))},
	// Misaligned CIDR
	{&MasqConfig{NonMasqueradeCIDRs: []string{"10.0.0.1/8"}}, fmt.Errorf(cidrAlignErrFmt, "10.0.0.1/8", "10.0.0.1", "10.0.0.0/8")},
}

// tests the MasqConfig.validate method
func TestConfigValidate(t *testing.T) {
	for _, tt := range validateConfigTests {
		err := tt.cfg.validate()
		if errorToString(err) != errorToString(tt.err) {
			t.Errorf("%+v.validate() => %s, want %s", tt.cfg, errorToString(err), errorToString(tt.err))
		}
	}
}

// specs for testing loading config from fs
var syncConfigTests = []struct {
	desc string            // human readable description of the fs used for the test e.g. "no config file"
	fs   fakefs.FileSystem // filesystem interface
	err  error             // expected error from MasqDaemon.syncConfig(fs)
	cfg  *MasqConfig       // expected values of the configuration after loading from fs
}{
	// happy paths

	{"single valid yaml file, full keys",
		fakefs.StringFS{Files: []fakefs.File{{
			Name: configFilePrefix + "-config-0",
			Path: configPath,
			Content: `
nonMasqueradeCIDRs:
  - 172.16.0.0/12
  - 10.0.0.0/8
masqLinkLocal: true
masqLinkLocalIPv6: true
`}}}, nil, &MasqConfig{
			NonMasqueradeCIDRs: []string{"172.16.0.0/12", "10.0.0.0/8"},
			MasqLinkLocal:      true,
			MasqLinkLocalIPv6:  true}},

	{"single valid yaml file, just nonMasqueradeCIDRs", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
nonMasqueradeCIDRs:
  - 192.168.0.0/16
`}}}, nil, &MasqConfig{
		NonMasqueradeCIDRs: []string{"192.168.0.0/16"},
		MasqLinkLocal:      NewMasqConfigNoReservedRanges().MasqLinkLocal}},

	{"single valid yaml file, just masqLinkLocal", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
masqLinkLocal: true
`}}}, nil, &MasqConfig{
		NonMasqueradeCIDRs: []string{},
		MasqLinkLocal:      true}},

	{"single valid json file, partial keys", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
		{
		  "nonMasqueradeCIDRs": ["172.16.0.0/12", "10.0.0.0/8"],
		  "masqLinkLocal": true
		}
`}}}, nil, &MasqConfig{
		NonMasqueradeCIDRs: []string{"172.16.0.0/12", "10.0.0.0/8"},
		MasqLinkLocal:      true}},

	{"single valid json file, just nonMasqueradeCIDRs", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
		{
			"nonMasqueradeCIDRs": ["192.168.0.0/16"]
		}
`}}}, nil, &MasqConfig{
		NonMasqueradeCIDRs: []string{"192.168.0.0/16"},
		MasqLinkLocal:      NewMasqConfigNoReservedRanges().MasqLinkLocal}},

	{"single valid json file, just masqLinkLocal", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
		{
			"masqLinkLocal": true
		}
`}}}, nil, &MasqConfig{
		NonMasqueradeCIDRs: []string{},
		MasqLinkLocal:      true}},

	{"single valid json file, all keys with ipv6 cidr", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
		{
		  "nonMasqueradeCIDRs": ["172.16.0.0/12", "10.0.0.0/8", "fc00::/7"],
		  "masqLinkLocal": true,
          "masqLinkLocalIPv6": true
		}
		`}}}, nil, &MasqConfig{
		NonMasqueradeCIDRs: []string{"172.16.0.0/12", "10.0.0.0/8", "fc00::/7"},
		MasqLinkLocal:      true,
		MasqLinkLocalIPv6:  true}},

	{"multiple valid json files, full keys, ipv6 cidr", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
		{
		  "nonMasqueradeCIDRs": ["172.16.0.0/12", "10.0.0.0/8", "fc00::/7"],
		  "masqLinkLocal": false,
		  "masqLinkLocalIPv6": false
		}
		`}, {
		Name: configFilePrefix + "config-1",
		Path: configPath,
		Content: `
		{
		  "nonMasqueradeCIDRs": ["1.0.0.0/8", "2.2.0.0/16"],
		  "masqLinkLocal": true,
		  "masqLinkLocalIPv6": false
		}
		`},
	}}, nil, &MasqConfig{
		NonMasqueradeCIDRs: []string{"172.16.0.0/12", "10.0.0.0/8", "fc00::/7", "1.0.0.0/8", "2.2.0.0/16"},
		MasqLinkLocal:      true,
		MasqLinkLocalIPv6:  false}},

	{"multiple valid json files, but one not correctly prefixed", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
		{
		  "nonMasqueradeCIDRs": ["111.254.0.0/15", "8.0.0.0/8"],
		  "masqLinkLocal": false,
		  "masqLinkLocalIPv6": false
		}
		`}, {
		Name: "bad-prefix-config-1",
		Path: configPath,
		Content: `
		{
		  "nonMasqueradeCIDRs": ["4.4.0.0/8", "6.7.0.0/16"],
		  "masqLinkLocal": true,
		  "masqLinkLocalIPv6": true
		}
		`},
	}}, nil, &MasqConfig{
		NonMasqueradeCIDRs: []string{"111.254.0.0/15", "8.0.0.0/8"},
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  false}},

	{"multiple valid yaml files, partial keys, duplicate cidr", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
nonMasqueradeCIDRs:
  - 155.128.0.0/9
  - 10.0.0.0/8
masqLinkLocal: false
`}, {
		Name: configFilePrefix + "config-1",
		Path: configPath,
		Content: `
masqLinkLocalIPv6: true
`}, {
		Name: configFilePrefix + "config-2",
		Path: configPath,
		Content: `
nonMasqueradeCIDRs:
  - 10.240.0.0/16
  - 10.0.0.0/8
  - 180.132.128.0/18
`},
	}}, nil, &MasqConfig{
		NonMasqueradeCIDRs: []string{"155.128.0.0/9", "10.0.0.0/8", "10.240.0.0/16", "180.132.128.0/18"},
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  true}},

	// sad paths

	{"no config file", fakefs.NotExistFS{}, fmt.Errorf("failed to read config directory, error: open /etc/config/: errno 2"), NewMasqConfigNoReservedRanges()}, // If the file does not exist, defaults should be used

	{"invalid json file", fakefs.StringFS{Files: []fakefs.File{{
		Name:    configFilePrefix + "-config-0",
		Path:    configPath,
		Content: `{*`}}}, fmt.Errorf("failed to unmarshal config file \"ip-masq-config-0\", error: invalid character '*' looking for beginning of object key string"), NewMasqConfigNoReservedRanges()},

	{"invalid yaml file", fakefs.StringFS{Files: []fakefs.File{{
		Name:    configFilePrefix + "-config-0",
		Path:    configPath,
		Content: `*`}}}, fmt.Errorf("failed to convert config file \"ip-masq-config-0\" to JSON, error: yaml: did not find expected alphabetic or numeric character"), NewMasqConfigNoReservedRanges()},

	{"no correctly prefixed file", fakefs.StringFS{Files: []fakefs.File{{
		Name: "bad-prefix-config-0",
		Path: configPath,
		Content: `
		{
		  "nonMasqueradeCIDRs": ["225.255.240.0/20"],
		  "masqLinkLocal": false,
		  "masqLinkLocalIPv6": false
		}
		`},
	}}, nil, NewMasqConfigNoReservedRanges()},

	{"no file in correct directory", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: "",
		Content: `
		{
		  "nonMasqueradeCIDRs": ["225.128.0.0/9"],
		  "masqLinkLocal": false,
		  "masqLinkLocalIPv6": false
		}
		`},
	}}, fmt.Errorf("failed to read config file \"ip-masq-config-0\", error: open " + filepath.Join(configPath, configFilePrefix+"-config-0") + ": errno 2"), NewMasqConfigNoReservedRanges()},

	{"single invalid yaml file, empty entry",
		fakefs.StringFS{Files: []fakefs.File{{
			Name: configFilePrefix + "-config-0",
			Path: configPath,
			Content: `
nonMasqueradeCIDRs:
  - 
`}}}, fmt.Errorf("config ip-masq-config-0 is invalid: CIDR \"\" could not be parsed, invalid CIDR address: "), NewMasqConfigNoReservedRanges()},

	{"single invalid yaml file, mix valid and empty values",
		fakefs.StringFS{Files: []fakefs.File{{
			Name: configFilePrefix + "-config-0",
			Path: configPath,
			Content: `
nonMasqueradeCIDRs:
  - 192.168.0.0/24
  - 
  - fd88:1234::/80
masqLinkLocal: true
masqLinkLocalIPv6: true
`}}}, fmt.Errorf("config ip-masq-config-0 is invalid: CIDR \"\" could not be parsed, invalid CIDR address: "), NewMasqConfigNoReservedRanges()},

	{"single invalid yaml file, whitespace",
		fakefs.StringFS{Files: []fakefs.File{{
			Name: configFilePrefix + "-config-0",
			Path: configPath,
			Content: `
nonMasqueradeCIDRs:
  - ""
`}}}, fmt.Errorf("config ip-masq-config-0 is invalid: CIDR \"\" could not be parsed, invalid CIDR address: "), NewMasqConfigNoReservedRanges()},

	{"multiple yaml configs, one bad config - empty CIDR value",
		fakefs.StringFS{Files: []fakefs.File{{
			Name: configFilePrefix + "-config-0",
			Path: configPath,
			Content: `
nonMasqueradeCIDRs:
  -
`},
			{
				Name: configFilePrefix + "-config-1",
				Path: configPath,
				Content: `
nonMasqueradeCIDRs:
  - 192.168.0.0/24
masqLinkLocal: true
masqLinkLocalIPv6: true
`}}}, fmt.Errorf("config ip-masq-config-0 is invalid: CIDR \"\" could not be parsed, invalid CIDR address: "), NewMasqConfigNoReservedRanges()},

	{"multiple json files, but one has empty CIDR", fakefs.StringFS{Files: []fakefs.File{{
		Name: configFilePrefix + "-config-0",
		Path: configPath,
		Content: `
	{
	  "nonMasqueradeCIDRs": ["111.254.0.0/15", "8.0.0.0/8"],
	  "masqLinkLocal": false,
	  "masqLinkLocalIPv6": false
	}
	`}, {
		Name: configFilePrefix + "-config-1",
		Path: configPath,
		Content: `
	{
	  "nonMasqueradeCIDRs": [null, "172.168.0.0/16"],
	  "masqLinkLocal": true,
	  "masqLinkLocalIPv6": true
	}
	`},
	}}, fmt.Errorf("config ip-masq-config-1 is invalid: CIDR \"\" could not be parsed, invalid CIDR address: "), NewMasqConfigNoReservedRanges()},
}

// tests MasqDaemon.syncConfig
func TestSyncConfig(t *testing.T) {
	for _, tt := range syncConfigTests {
		_ = flag.Set("enable-ipv6", "true")
		m := NewFakeMasqDaemon()
		m.config = NewMasqConfigNoReservedRanges()
		err := m.syncConfig(tt.fs)
		if errorToString(err) != errorToString(tt.err) {
			t.Errorf("MasqDaemon.syncConfig(fs: %s) => %s, want %s", tt.desc, errorToString(err), errorToString(tt.err))
		} else if !slicesEqual(m.config.NonMasqueradeCIDRs, tt.cfg.NonMasqueradeCIDRs) ||
			m.config.MasqLinkLocal != tt.cfg.MasqLinkLocal ||
			m.config.MasqLinkLocalIPv6 != tt.cfg.MasqLinkLocalIPv6 {
			t.Errorf("MasqDaemon.syncConfig(fs: %s) loaded as %+v, want %+v", tt.desc, m.config, tt.cfg)
		}
	}
}

// tests MasqDaemon.syncMasqRules
func TestSyncMasqRules(t *testing.T) {
	var syncMasqRulesTests = []struct {
		desc string      // human readable description of the test
		cfg  *MasqConfig // Masq configuration to use
		err  error       // expected error, if any. If nil, no error expected
		want string      // String expected to be sent to iptables-restore
	}{
		{
			desc: "empty config",
			cfg:  &MasqConfig{},
			want: `*nat
:` + string(masqChain) + ` - [0:0]
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 169.254.0.0/16 -j RETURN
-A ` + string(masqChain) + ` ` + masqRuleComment + ` -j MASQUERADE
COMMIT
`,
		},
		{
			desc: "default config masquerading reserved ranges",
			cfg:  NewMasqConfigNoReservedRanges(),
			want: `*nat
:` + string(masqChain) + ` - [0:0]
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 169.254.0.0/16 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 10.0.0.0/8 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 172.16.0.0/12 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 192.168.0.0/16 -j RETURN
-A ` + string(masqChain) + ` ` + masqRuleComment + ` -j MASQUERADE
COMMIT
`,
		},
		{
			desc: "default config not masquerading reserved ranges",
			cfg:  NewMasqConfigWithReservedRanges(),
			want: `*nat
:` + string(masqChain) + ` - [0:0]
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 169.254.0.0/16 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 10.0.0.0/8 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 172.16.0.0/12 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 192.168.0.0/16 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 100.64.0.0/10 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 192.0.0.0/24 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 192.0.2.0/24 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 192.88.99.0/24 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 198.18.0.0/15 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 198.51.100.0/24 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 203.0.113.0/24 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 240.0.0.0/4 -j RETURN
-A ` + string(masqChain) + ` ` + masqRuleComment + ` -j MASQUERADE
COMMIT
`,
		},
		{
			desc: "config has ipv4 and ipv6 non masquerade cidr",
			cfg: &MasqConfig{
				NonMasqueradeCIDRs: []string{
					"10.244.0.0/16",
					"fc00::/7",
				},
			},
			want: `*nat
:` + string(masqChain) + ` - [0:0]
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 169.254.0.0/16 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d 10.244.0.0/16 -j RETURN
-A ` + string(masqChain) + ` ` + masqRuleComment + ` -j MASQUERADE
COMMIT
`,
		},
	}

	for _, tt := range syncMasqRulesTests {
		t.Run(tt.desc, func(t *testing.T) {
			m := NewFakeMasqDaemon()
			m.config = tt.cfg
			err := m.syncMasqRules()
			if err != nil {
				t.Errorf("syncMasqRules failed, error: %v", err)
			}
			fipt, ok := m.iptables.(*iptest.FakeIPTables)
			if !ok {
				t.Errorf("MasqDaemon wasn't using the expected iptables mock")
			}
			if string(fipt.Lines) != tt.want {
				t.Errorf("syncMasqRules wrote %q, want %q", string(fipt.Lines), tt.want)
			}
		})
	}
}

// tests MasqDaemon.syncMasqRulesIPv6
func TestSyncMasqRulesIPv6(t *testing.T) {
	var syncMasqRulesIPv6Tests = []struct {
		desc string      // human readable description of the test
		cfg  *MasqConfig // Masq configuration to use
		err  error       // expected error, if any. If nil, no error expected
		want string      // String expected to be sent to iptables-restore
	}{
		{
			desc: "empty config",
			cfg:  &MasqConfig{},
			want: `*nat
:` + string(masqChain) + ` - [0:0]
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d fe80::/10 -j RETURN
-A ` + string(masqChain) + ` ` + masqRuleComment + ` -j MASQUERADE
COMMIT
`,
		},
		{
			desc: "config has ipv4 and ipv6 non masquerade cidr",
			cfg: &MasqConfig{
				NonMasqueradeCIDRs: []string{
					"10.244.0.0/16",
					"fc00::/7",
				},
			},
			want: `*nat
:` + string(masqChain) + ` - [0:0]
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d fe80::/10 -j RETURN
-A ` + string(masqChain) + ` ` + nonMasqRuleComment + ` -d fc00::/7 -j RETURN
-A ` + string(masqChain) + ` ` + masqRuleComment + ` -j MASQUERADE
COMMIT
`,
		},
		{
			desc: "config has masqLinkLocalIPv6: true",
			cfg:  &MasqConfig{MasqLinkLocalIPv6: true},
			want: `*nat
:` + string(masqChain) + ` - [0:0]
-A ` + string(masqChain) + ` ` + masqRuleComment + ` -j MASQUERADE
COMMIT
`,
		},
	}

	for _, tt := range syncMasqRulesIPv6Tests {
		t.Run(tt.desc, func(t *testing.T) {
			_ = flag.Set("enable-ipv6", "true")
			m := NewFakeMasqDaemon()
			m.config = tt.cfg
			err := m.syncMasqRulesIPv6()
			if err != nil {
				t.Errorf("syncMasqRules failed, error: %v", err)
			}
			fipt6, ok := m.ip6tables.(*iptest.FakeIPTables)
			if !ok {
				t.Errorf("MasqDaemon wasn't using the expected iptables mock")
			}
			if string(fipt6.Lines) != tt.want {
				t.Errorf("syncMasqRulesIPv6 wrote %q, want %q", string(fipt6.Lines), tt.want)
			}
		})
	}
}

// TODO(mtaufen): switch to an iptables mock that allows us to check the results of EnsureRule
// tests m.ensurePostroutingJump
func TestEnsurePostroutingJump(t *testing.T) {
	m := NewFakeMasqDaemon()
	err := m.ensurePostroutingJump()
	if err != nil {
		t.Errorf("error: %v", err)
	}
}

// tests writeNonMasqRule
func TestWriteNonMasqRule(t *testing.T) {
	var writeNonMasqRuleTests = []struct {
		desc string
		cidr string
		want string
	}{
		{
			desc: "with ipv4 non masquerade cidr",
			cidr: "10.0.0.0/8",
			want: string(utiliptables.Append) + " " + string(masqChain) +
				` -m comment --comment "ip-masq-agent: local traffic is not subject to MASQUERADE"` +
				" -d 10.0.0.0/8 -j RETURN\n",
		},
		{
			desc: "with ipv6 non masquerade cidr",
			cidr: "fc00::/7",
			want: string(utiliptables.Append) + " " + string(masqChain) +
				` -m comment --comment "ip-masq-agent: local traffic is not subject to MASQUERADE"` +
				" -d fc00::/7 -j RETURN\n",
		},
	}

	for _, tt := range writeNonMasqRuleTests {
		t.Run(tt.desc, func(t *testing.T) {
			lines := bytes.NewBuffer(nil)
			writeNonMasqRule(lines, tt.cidr)

			s, err := lines.ReadString('\n')
			if err != nil {
				t.Error("writeRule did not append a newline")
			}
			if s != tt.want {
				t.Errorf("writeNonMasqRule(lines, "+tt.cidr+"):\n   got: %q\n  want: %q", s, tt.want)
			}
		})
	}
}

// tests writeRule
func TestWriteRule(t *testing.T) {
	lines := bytes.NewBuffer(nil)
	want := string(utiliptables.Append) + " " + string(masqChain) +
		" -m comment --comment \"test writing a rule\"\n"
	writeRule(lines, utiliptables.Append, masqChain, "-m", "comment", "--comment", `"test writing a rule"`)

	s, err := lines.ReadString('\n')
	if err != nil {
		t.Error("writeRule did not append a newline")
	}
	if s != want {
		t.Errorf("writeRule(lines, pos, chain, \"-m\", \"comment\", \"--comment\", `\"test writing a rule\"`) wrote %q, want %q", s, want)
	}
}

// tests writeLine
func TestWriteLine(t *testing.T) {
	lines := bytes.NewBuffer(nil)
	want := "a b c\n"

	writeLine(lines, "a", "b", "c")

	s, err := lines.ReadString('\n')
	if err != nil {
		t.Error("writeLine did not append a newline")
	}
	if s != want {
		t.Errorf("writeLine(lines, \"a\", \"b\", \"c\") wrote %q, want %q", s, want)
	}
}

// convert error to string, while also handling nil errors
func errorToString(err error) string {
	if err == nil {
		return "nil error"
	}
	return fmt.Sprintf("error %q", err.Error())
}

// tests merge both full
func TestMerge(t *testing.T) {
	c := EmptyMasqConfig()

	c1 := &MasqConfig{
		NonMasqueradeCIDRs: []string{"155.128.0.0/9", "10.240.0.0/16", "180.132.128.0/18", "3.3.3.0/24"},
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  true,
	}

	want := &MasqConfig{
		NonMasqueradeCIDRs: []string{"155.128.0.0/9", "10.240.0.0/16", "180.132.128.0/18", "3.3.3.0/24"},
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  true,
	}

	c.merge(c1)
	if !c.equals(want) {
		t.Errorf("c.merge(c1) wrote %v, want %v", c, want)
	}

	c2 := &MasqConfig{
		NonMasqueradeCIDRs: []string{"1.0.0.0/8", "2.2.0.0/16", "3.3.3.0/24"},
		MasqLinkLocal:      true,
		MasqLinkLocalIPv6:  false,
	}

	want = &MasqConfig{
		NonMasqueradeCIDRs: []string{"155.128.0.0/9", "10.240.0.0/16", "180.132.128.0/18", "3.3.3.0/24", "1.0.0.0/8", "2.2.0.0/16"},
		MasqLinkLocal:      true,
		MasqLinkLocalIPv6:  true,
	}

	c.merge(c2)
	if !c.equals(want) {
		t.Errorf("c.merge(c2) wrote %v, want %v", c, want)
	}
}

// tests merge handling missing fields
func TestMergeIncomplete(t *testing.T) {
	c := EmptyMasqConfig()

	c1 := &MasqConfig{
		NonMasqueradeCIDRs: []string{"10.128.0.192/26", "100.16.0.0/12"},
	}

	c.merge(c1)

	want := &MasqConfig{
		NonMasqueradeCIDRs: []string{"10.128.0.192/26", "100.16.0.0/12"},
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  false,
	}

	c.merge(c1)
	if !c.equals(want) {
		t.Errorf("c.merge(c1) [missing fields] wrote %v, want %v", c, want)
	}

	c2 := &MasqConfig{
		MasqLinkLocal: true,
	}

	want = &MasqConfig{
		NonMasqueradeCIDRs: []string{"10.128.0.192/26", "100.16.0.0/12"},
		MasqLinkLocal:      true,
		MasqLinkLocalIPv6:  false,
	}

	c.merge(c2)
	if !c.equals(want) {
		t.Errorf("c.merge(c2) [missing fields] wrote %v, want %v", c, want)
	}
}

func (c *MasqConfig) equals(newConfig *MasqConfig) bool {
	return slicesEqual(c.NonMasqueradeCIDRs, newConfig.NonMasqueradeCIDRs) &&
		c.MasqLinkLocal == newConfig.MasqLinkLocal &&
		c.MasqLinkLocalIPv6 == newConfig.MasqLinkLocalIPv6
}

// Ignore ordering, just check if size and elements are the same
func slicesEqual(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}

	xMap := make(map[string]int)
	yMap := make(map[string]int)

	for _, xElem := range x {
		xMap[xElem]++
	}
	for _, yElem := range y {
		yMap[yElem]++
	}

	for xMapKey, xMapVal := range xMap {
		if yMap[xMapKey] != xMapVal {
			return false
		}
	}
	return true
}
