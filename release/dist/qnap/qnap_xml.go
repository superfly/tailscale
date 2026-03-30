// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package qnap

import (
	"encoding/xml"
	"fmt"
	"time"
)

// platformIDs maps QNAP build architecture names to the set of QNAP
// platform IDs that should be served that architecture's QPKG.
//
// QNAP devices identify themselves by a platform ID when checking
// the App Center for available packages. Multiple platform IDs
// can map to the same binary architecture (e.g. many ARM64 NAS
// models all use the same arm_64 QPKG).
//
// This mapping is reverse-engineered from third-party QNAP
// repositories (myqnap.org, qnapclub.eu) and the QDK source code.
// QNAP does not publish an official specification.
var platformIDs = map[string][]string{
	"x86_64": {"TS-NASX86"},
	"arm_64": {
		"TS-NASARM_64",
		"TS-X16",
		"TS-X28A",
		"TS-X32", "TS-X32U",
		"TS-X33", "TS-X33EU", "TS-X33U",
		"TS-X35", "TS-X35A", "TS-X35EU",
		"TS-X42",
		"TS-XA28A",
	},
	"arm-x41": {
		"TS-X41",
		"TS-X31P2", "TS-X31P3",
		"TS-X31X", "TS-X31XU",
		"TS-531P",
	},
	"arm-x31": {"TS-X31"},
	"arm-x19": {"TS-ARM-X19", "TS-ARM-X09"},
	// x86 and x86_ce53xx both target 32-bit Intel NAS devices.
	// In the repo XML we list only the x86 QPKG under
	// TS-NASX86_OLD32 to avoid duplicate platform entries.
	// The x86_ce53xx QPKG remains available for direct download
	// from pkgs.tailscale.com.
	"x86": {"TS-NASX86_OLD32"},
}

// archesInRepoXML is the ordered list of architectures included in
// the repository XML. x86_ce53xx is excluded because it shares the
// TS-NASX86_OLD32 platform ID with x86.
var archesInRepoXML = []string{
	"x86_64",
	"arm_64",
	"arm-x41",
	"arm-x31",
	"arm-x19",
	"x86",
}

// repoXML is the root element of a QNAP App Center repository XML file.
type repoXML struct {
	XMLName  xml.Name      `xml:"plugins"`
	CacheChk string        `xml:"cachechk"`
	Items    []repoXMLItem `xml:"item"`
}

// repoXMLItem represents a single package in the repository.
type repoXMLItem struct {
	Name          string            `xml:"name"`
	InternalName  string            `xml:"internalName"`
	ChangeLog     string            `xml:"changeLog"`
	Category      string            `xml:"category"`
	Type          string            `xml:"type"`
	Icon80        string            `xml:"icon80"`
	Icon100       string            `xml:"icon100"`
	Description   string            `xml:"description"`
	Version       string            `xml:"version"`
	Platforms     []repoXMLPlatform `xml:"platform"`
	PublishedDate string            `xml:"publishedDate"`
	Maintainer    string            `xml:"maintainer"`
	Developer     string            `xml:"developer"`
	ForumLink     string            `xml:"forumlink"`
	Language      string            `xml:"language"`
	Snapshot      string            `xml:"snapshot"`
	BannerImg     string            `xml:"bannerImg"`
	TutorialLink  string            `xml:"tutorialLink"`
}

// repoXMLPlatform identifies a downloadable QPKG for a specific hardware platform.
type repoXMLPlatform struct {
	PlatformID string `xml:"platformID"`
	Location   string `xml:"location"`
	Signature  string `xml:"signature"`
}

// RepoXMLParams contains the parameters needed to generate a QNAP
// repository XML file.
type RepoXMLParams struct {
	// Version is the Tailscale version string (e.g. "1.96.2").
	Version string

	// BaseURL is the base URL where QPKGs are hosted, with a
	// trailing slash (e.g. "https://pkgs.tailscale.com/stable/").
	BaseURL string

	// QPKGs maps architecture names (e.g. "x86_64", "arm_64") to
	// QPKG filenames (e.g. "Tailscale_1.96.2-1_x86_64.qpkg").
	// Only architectures present in this map are included in the XML.
	QPKGs map[string]string

	// Signatures maps architecture names to their codesigning
	// signature strings. If empty or missing for an architecture,
	// an empty signature is used.
	Signatures map[string]string

	// Timestamp is the build time, used for the cachechk field
	// and publishedDate.
	Timestamp time.Time

	// IconBaseURL is the base URL where icon files are hosted.
	// If empty, defaults to BaseURL.
	IconBaseURL string
}

const (
	tailscaleDescription = "Tailscale lets you easily manage access to private resources, quickly SSH into devices on your network, and work securely from anywhere in the world."
	tailscaleChangeLog   = "https://tailscale.com/changelog"
)

// GenerateRepoXML produces a complete QNAP App Center repository XML
// document for the given parameters.
//
// The XML contains a single <item> for Tailscale, with <platform>
// entries expanded for all known QNAP platform IDs that map to the
// provided architectures.
func GenerateRepoXML(p RepoXMLParams) ([]byte, error) {
	if p.Version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if p.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if len(p.QPKGs) == 0 {
		return nil, fmt.Errorf("at least one QPKG is required")
	}

	iconBase := p.IconBaseURL
	if iconBase == "" {
		iconBase = p.BaseURL
	}

	var platforms []repoXMLPlatform
	for _, arch := range archesInRepoXML {
		filename, ok := p.QPKGs[arch]
		if !ok {
			continue
		}
		pids, ok := platformIDs[arch]
		if !ok {
			continue
		}
		sig := p.Signatures[arch]
		location := p.BaseURL + filename
		for _, pid := range pids {
			platforms = append(platforms, repoXMLPlatform{
				PlatformID: pid,
				Location:   location,
				Signature:  sig,
			})
		}
	}

	if len(platforms) == 0 {
		return nil, fmt.Errorf("no platforms matched the provided QPKGs")
	}

	doc := repoXML{
		CacheChk: p.Timestamp.UTC().Format("200601021504"),
		Items: []repoXMLItem{
			{
				Name:          "Tailscale",
				InternalName:  "Tailscale",
				ChangeLog:     tailscaleChangeLog,
				Category:      "Tailscale",
				Type:          "Networking",
				Icon80:        iconBase + "tailscale-icon-80.gif",
				Icon100:       iconBase + "tailscale-icon-100.png",
				Description:   tailscaleDescription,
				Version:       p.Version,
				Platforms:     platforms,
				PublishedDate: p.Timestamp.UTC().Format("2006-01-02 15:04:05"),
				Maintainer:    "Tailscale Inc.",
				Developer:     "Tailscale Inc.",
				Language:      "English",
			},
		},
	}

	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling XML: %w", err)
	}

	return append([]byte(xml.Header), out...), nil
}
