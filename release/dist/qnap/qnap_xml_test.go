// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package qnap

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestGenerateRepoXML(t *testing.T) {
	ts := time.Date(2026, 3, 30, 14, 30, 0, 0, time.UTC)

	got, err := GenerateRepoXML(RepoXMLParams{
		Version: "1.98.0",
		BaseURL: "https://pkgs.tailscale.com/stable/",
		QPKGs: map[string]string{
			"x86_64":  "Tailscale_1.98.0-1_x86_64.qpkg",
			"arm_64":  "Tailscale_1.98.0-1_arm_64.qpkg",
			"arm-x41": "Tailscale_1.98.0-1_arm-x41.qpkg",
			"arm-x31": "Tailscale_1.98.0-1_arm-x31.qpkg",
			"arm-x19": "Tailscale_1.98.0-1_arm-x19.qpkg",
			"x86":     "Tailscale_1.98.0-1_x86.qpkg",
		},
		Signatures: map[string]string{
			"x86_64": "abc123signature+base64value==",
		},
		Timestamp: ts,
	})
	if err != nil {
		t.Fatal(err)
	}

	xmlStr := string(got)

	// Verify XML declaration.
	if !strings.HasPrefix(xmlStr, xml.Header) {
		t.Errorf("missing or wrong XML declaration, starts with: %.50s", xmlStr)
	}

	// Verify it round-trips through the XML parser.
	var parsed repoXML
	if err := xml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("generated XML does not parse: %v", err)
	}

	// Verify cachechk timestamp format.
	if parsed.CacheChk != "202603301430" {
		t.Errorf("cachechk = %q, want %q", parsed.CacheChk, "202603301430")
	}

	// Verify single item.
	if len(parsed.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(parsed.Items))
	}
	item := parsed.Items[0]

	if item.Name != "Tailscale" {
		t.Errorf("name = %q, want %q", item.Name, "Tailscale")
	}
	if item.InternalName != "Tailscale" {
		t.Errorf("internalName = %q, want %q", item.InternalName, "Tailscale")
	}
	if item.Version != "1.98.0" {
		t.Errorf("version = %q, want %q", item.Version, "1.98.0")
	}
	if item.Category != "Tailscale" {
		t.Errorf("category = %q, want %q", item.Category, "Tailscale")
	}
	if item.Type != "Networking" {
		t.Errorf("type = %q, want %q", item.Type, "Networking")
	}
	if item.PublishedDate != "2026-03-30 14:30:00" {
		t.Errorf("publishedDate = %q, want %q", item.PublishedDate, "2026-03-30 14:30:00")
	}
	if item.Maintainer != "Tailscale Inc." {
		t.Errorf("maintainer = %q", item.Maintainer)
	}
	if item.Language != "English" {
		t.Errorf("language = %q", item.Language)
	}
	if item.Description == "" {
		t.Error("description is empty")
	}

	// Verify icons use the base URL.
	if item.Icon80 != "https://pkgs.tailscale.com/stable/tailscale-icon-80.gif" {
		t.Errorf("icon80 = %q", item.Icon80)
	}
	if item.Icon100 != "https://pkgs.tailscale.com/stable/tailscale-icon-100.png" {
		t.Errorf("icon100 = %q", item.Icon100)
	}

	// Verify platforms are expanded correctly.
	// 1 (x86_64) + 13 (arm_64) + 6 (arm-x41) + 1 (arm-x31) + 2 (arm-x19) + 1 (x86) = 24
	wantPlatforms := 24
	if len(item.Platforms) != wantPlatforms {
		t.Errorf("got %d platforms, want %d", len(item.Platforms), wantPlatforms)
	}

	findPlatform := func(pid string) *repoXMLPlatform {
		for i := range item.Platforms {
			if item.Platforms[i].PlatformID == pid {
				return &item.Platforms[i]
			}
		}
		return nil
	}

	// x86_64 should have a signature.
	p := findPlatform("TS-NASX86")
	if p == nil {
		t.Fatal("TS-NASX86 platform not found")
	}
	if p.Location != "https://pkgs.tailscale.com/stable/Tailscale_1.98.0-1_x86_64.qpkg" {
		t.Errorf("TS-NASX86 location = %q", p.Location)
	}
	if p.Signature != "abc123signature+base64value==" {
		t.Errorf("TS-NASX86 signature = %q", p.Signature)
	}

	// ARM64 model-specific IDs should all point to the same QPKG.
	for _, pid := range []string{"TS-NASARM_64", "TS-X16", "TS-X28A", "TS-X42"} {
		p := findPlatform(pid)
		if p == nil {
			t.Errorf("%s platform not found", pid)
			continue
		}
		if p.Location != "https://pkgs.tailscale.com/stable/Tailscale_1.98.0-1_arm_64.qpkg" {
			t.Errorf("%s location = %q, want arm_64 QPKG", pid, p.Location)
		}
		if p.Signature != "" {
			t.Errorf("%s signature = %q, want empty", pid, p.Signature)
		}
	}

	// arm-x41 platforms.
	for _, pid := range []string{"TS-X41", "TS-X31P2", "TS-531P"} {
		p := findPlatform(pid)
		if p == nil {
			t.Errorf("%s platform not found", pid)
			continue
		}
		if !strings.HasSuffix(p.Location, "arm-x41.qpkg") {
			t.Errorf("%s location = %q, want arm-x41 QPKG", pid, p.Location)
		}
	}

	// x86 32-bit.
	p = findPlatform("TS-NASX86_OLD32")
	if p == nil {
		t.Fatal("TS-NASX86_OLD32 platform not found")
	}
	if !strings.HasSuffix(p.Location, "x86.qpkg") {
		t.Errorf("TS-NASX86_OLD32 location = %q, want x86 QPKG", p.Location)
	}
}

func TestGenerateRepoXMLCustomIconBase(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got, err := GenerateRepoXML(RepoXMLParams{
		Version: "1.98.0",
		BaseURL: "https://pkgs.tailscale.com/unstable/",
		QPKGs: map[string]string{
			"x86_64": "Tailscale_1.98.0-1_x86_64.qpkg",
		},
		Timestamp:   ts,
		IconBaseURL: "https://cdn.tailscale.com/icons/",
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed repoXML
	if err := xml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("generated XML does not parse: %v", err)
	}

	item := parsed.Items[0]
	if item.Icon80 != "https://cdn.tailscale.com/icons/tailscale-icon-80.gif" {
		t.Errorf("icon80 = %q", item.Icon80)
	}
	if item.Icon100 != "https://cdn.tailscale.com/icons/tailscale-icon-100.png" {
		t.Errorf("icon100 = %q", item.Icon100)
	}
}

func TestGenerateRepoXMLMissingArch(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got, err := GenerateRepoXML(RepoXMLParams{
		Version: "1.98.0",
		BaseURL: "https://pkgs.tailscale.com/stable/",
		QPKGs: map[string]string{
			"x86_64": "Tailscale_1.98.0-1_x86_64.qpkg",
		},
		Timestamp: ts,
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed repoXML
	if err := xml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("generated XML does not parse: %v", err)
	}

	if len(parsed.Items[0].Platforms) != 1 {
		t.Errorf("got %d platforms, want 1", len(parsed.Items[0].Platforms))
	}
	if parsed.Items[0].Platforms[0].PlatformID != "TS-NASX86" {
		t.Errorf("platformID = %q, want TS-NASX86", parsed.Items[0].Platforms[0].PlatformID)
	}
}

func TestGenerateRepoXMLIgnoresUnknownArch(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got, err := GenerateRepoXML(RepoXMLParams{
		Version: "1.98.0",
		BaseURL: "https://pkgs.tailscale.com/stable/",
		QPKGs: map[string]string{
			"x86_64":     "Tailscale_1.98.0-1_x86_64.qpkg",
			"x86_ce53xx": "Tailscale_1.98.0-1_x86_ce53xx.qpkg",
		},
		Timestamp: ts,
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed repoXML
	if err := xml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("generated XML does not parse: %v", err)
	}

	if len(parsed.Items[0].Platforms) != 1 {
		t.Errorf("got %d platforms, want 1 (x86_ce53xx should be excluded)", len(parsed.Items[0].Platforms))
	}
}

func TestGenerateRepoXMLErrors(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		params RepoXMLParams
	}{
		{
			name: "missing version",
			params: RepoXMLParams{
				BaseURL:   "https://example.com/",
				QPKGs:     map[string]string{"x86_64": "test.qpkg"},
				Timestamp: ts,
			},
		},
		{
			name: "missing base URL",
			params: RepoXMLParams{
				Version:   "1.0.0",
				QPKGs:     map[string]string{"x86_64": "test.qpkg"},
				Timestamp: ts,
			},
		},
		{
			name: "no QPKGs",
			params: RepoXMLParams{
				Version:   "1.0.0",
				BaseURL:   "https://example.com/",
				QPKGs:     map[string]string{},
				Timestamp: ts,
			},
		},
		{
			name: "only unknown arches",
			params: RepoXMLParams{
				Version:   "1.0.0",
				BaseURL:   "https://example.com/",
				QPKGs:     map[string]string{"mips64": "test.qpkg"},
				Timestamp: ts,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GenerateRepoXML(tt.params)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestPlatformIDMapping(t *testing.T) {
	for _, arch := range archesInRepoXML {
		pids, ok := platformIDs[arch]
		if !ok {
			t.Errorf("arch %q in archesInRepoXML but not in platformIDs", arch)
			continue
		}
		if len(pids) == 0 {
			t.Errorf("arch %q has empty platform ID list", arch)
		}
	}

	seen := map[string]string{}
	for _, arch := range archesInRepoXML {
		for _, pid := range platformIDs[arch] {
			if prev, ok := seen[pid]; ok {
				t.Errorf("platform ID %q appears under both %q and %q", pid, prev, arch)
			}
			seen[pid] = arch
		}
	}
}

func TestGenerateRepoXMLBaseURL(t *testing.T) {
	ts := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	qpkgs := map[string]string{
		"x86_64":  "Tailscale_1.100.0-1_x86_64.qpkg",
		"arm_64":  "Tailscale_1.100.0-1_arm_64.qpkg",
		"arm-x41": "Tailscale_1.100.0-1_arm-x41.qpkg",
		"arm-x31": "Tailscale_1.100.0-1_arm-x31.qpkg",
		"arm-x19": "Tailscale_1.100.0-1_arm-x19.qpkg",
		"x86":     "Tailscale_1.100.0-1_x86.qpkg",
	}

	for _, baseURL := range []string{
		"https://pkgs.tailscale.com/stable/",
		"https://pkgs.tailscale.com/unstable/",
	} {
		t.Run(baseURL, func(t *testing.T) {
			got, err := GenerateRepoXML(RepoXMLParams{
				Version:   "1.100.0",
				BaseURL:   baseURL,
				QPKGs:     qpkgs,
				Timestamp: ts,
			})
			if err != nil {
				t.Fatal(err)
			}

			var parsed repoXML
			if err := xml.Unmarshal(got, &parsed); err != nil {
				t.Fatalf("XML does not parse: %v", err)
			}

			if len(parsed.Items) != 1 {
				t.Fatalf("got %d items, want 1", len(parsed.Items))
			}

			for _, p := range parsed.Items[0].Platforms {
				if !strings.HasPrefix(p.Location, baseURL) {
					t.Errorf("platform %s location %q does not start with %s", p.PlatformID, p.Location, baseURL)
				}
			}
		})
	}
}
