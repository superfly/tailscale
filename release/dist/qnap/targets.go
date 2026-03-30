// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package qnap

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"tailscale.com/release/dist"
)

// Targets defines the dist.Targets for QNAP devices.
//
// If all parameters are provided non-empty, then the build will be signed using
// a Google Cloud hosted key.
//
// gcloudCredentialsBase64 is the JSON credential for connecting to Google Cloud, base64 encoded.
// gcloudKeyring is the full path to the Google Cloud keyring containing the signing key.
// keyName is the name of the key.
// certificateBase64 is the PEM certificate to use in the signature, base64 encoded.
func Targets(gcloudCredentialsBase64, gcloudProject, gcloudKeyring, keyName, certificateBase64, certificateIntermediariesBase64 string) []dist.Target {
	var signerInfo *signer
	if !slices.Contains([]string{gcloudCredentialsBase64, gcloudProject, gcloudKeyring, keyName, certificateBase64, certificateIntermediariesBase64}, "") {
		signerInfo = &signer{
			gcloudCredentialsBase64:         gcloudCredentialsBase64,
			gcloudProject:                   gcloudProject,
			gcloudKeyring:                   gcloudKeyring,
			keyName:                         keyName,
			certificateBase64:               certificateBase64,
			certificateIntermediariesBase64: certificateIntermediariesBase64,
		}
	}
	return []dist.Target{
		&target{
			arch: "x86",
			goenv: map[string]string{
				"GOOS":   "linux",
				"GOARCH": "386",
			},
			signer: signerInfo,
		},
		&target{
			arch: "x86_ce53xx",
			goenv: map[string]string{
				"GOOS":   "linux",
				"GOARCH": "386",
			},
			signer: signerInfo,
		},
		&target{
			arch: "x86_64",
			goenv: map[string]string{
				"GOOS":   "linux",
				"GOARCH": "amd64",
			},
			signer: signerInfo,
		},
		&target{
			arch: "arm-x31",
			goenv: map[string]string{
				"GOOS":   "linux",
				"GOARCH": "arm",
			},
			signer: signerInfo,
		},
		&target{
			arch: "arm-x41",
			goenv: map[string]string{
				"GOOS":   "linux",
				"GOARCH": "arm",
			},
			signer: signerInfo,
		},
		&target{
			arch: "arm-x19",
			goenv: map[string]string{
				"GOOS":   "linux",
				"GOARCH": "arm",
			},
			signer: signerInfo,
		},
		&target{
			arch: "arm_64",
			goenv: map[string]string{
				"GOOS":   "linux",
				"GOARCH": "arm64",
			},
			signer: signerInfo,
		},
	}
}

// goenvForArch maps QNAP build architecture names to the Go
// environment needed to cross-compile for that architecture.
var goenvForArch = map[string]map[string]string{
	"x86":     {"GOOS": "linux", "GOARCH": "386"},
	"x86_64":  {"GOOS": "linux", "GOARCH": "amd64"},
	"arm-x31": {"GOOS": "linux", "GOARCH": "arm"},
	"arm-x41": {"GOOS": "linux", "GOARCH": "arm"},
	"arm-x19": {"GOOS": "linux", "GOARCH": "arm"},
	"arm_64":  {"GOOS": "linux", "GOARCH": "arm64"},
}

// RepoXMLTarget returns a dist.Target that generates a QNAP App
// Center repository XML file.
//
// The target produces a qnap.xml file containing a single <item>
// for Tailscale with <platform> entries for all supported QNAP
// hardware platforms.
//
// The release track (stable/unstable) and base URL are derived
// automatically from b.Version.Track at build time.
//
// The XML target does not build QPKGs itself. It reads the QPKG
// filenames from the build output directory to construct the XML.
func RepoXMLTarget() dist.Target {
	return &repoXMLTarget{}
}

// repoXMLTarget is a dist.Target that generates a QNAP repository
// XML file.
type repoXMLTarget struct{}

func (t *repoXMLTarget) String() string {
	return "qnap/repo-xml"
}

func (t *repoXMLTarget) Build(b *dist.Build) ([]string, error) {
	baseURL := fmt.Sprintf("https://pkgs.tailscale.com/%s/", b.Version.Track)

	// Build all QPKGs through the shared memoized builder. If the
	// concurrent QPKG targets have already built (or are building)
	// a given arch, the memoize returns the same result without
	// duplicating work.
	qb := getQnapBuilds(b)

	qpkgs := make(map[string]string)
	sigs := make(map[string]string)
	for _, arch := range archesInRepoXML {
		goenv := goenvForArch[arch]
		if goenv == nil {
			log.Printf("qnap/repo-xml: no goenv for arch %s, skipping", arch)
			continue
		}
		outputs, err := qb.buildQPKG(b, arch, goenv)
		if err != nil {
			log.Printf("qnap/repo-xml: QPKG build for %s failed: %v, skipping", arch, err)
			continue
		}
		// First output is always the .qpkg file.
		filename := filepath.Base(outputs[0])
		qpkgs[arch] = filename

		// Read codesigning signature if available.
		sigPath := outputs[0] + ".codesigning"
		if sigData, err := os.ReadFile(sigPath); err == nil {
			sigs[arch] = strings.TrimSpace(string(sigData))
		}
	}

	if len(qpkgs) == 0 {
		return nil, fmt.Errorf("no QPKGs found in %s; QPKG targets must be built first", b.Out)
	}

	xmlData, err := GenerateRepoXML(RepoXMLParams{
		Version:    b.Version.Short,
		BaseURL:    baseURL,
		QPKGs:      qpkgs,
		Signatures: sigs,
		Timestamp:  b.Time,
	})
	if err != nil {
		return nil, fmt.Errorf("generating repo XML: %w", err)
	}

	outPath := filepath.Join(b.Out, "qnap.xml")
	if err := os.WriteFile(outPath, xmlData, 0644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", outPath, err)
	}
	log.Printf("qnap/repo-xml: wrote %s (%d bytes, %d architectures, track %s)",
		outPath, len(xmlData), len(qpkgs), b.Version.Track)

	outputs := []string{outPath}

	// Generate icon files for the QNAP App Center alongside the
	// XML. The 80x80 GIF is copied from the embedded build files
	// and the 100x100 PNG is scaled from the systray icon.
	iconOutputs, err := generateIcons(b)
	if err != nil {
		// Icon generation is best-effort; log but don't fail the build.
		log.Printf("qnap/repo-xml: warning: icon generation failed: %v", err)
	} else {
		outputs = append(outputs, iconOutputs...)
	}

	return outputs, nil
}

// generateIcons produces the icon files needed by the QNAP repo XML.
// Returns paths to the generated icon files.
func generateIcons(b *dist.Build) ([]string, error) {
	var outputs []string

	// Copy the existing 80x80 GIF from the embedded QNAP build files.
	gif80, err := buildFiles.ReadFile("files/Tailscale/icons/Tailscale_80.gif")
	if err != nil {
		return nil, fmt.Errorf("reading embedded 80x80 icon: %w", err)
	}
	gif80Path := filepath.Join(b.Out, "tailscale-icon-80.gif")
	if err := os.WriteFile(gif80Path, gif80, 0644); err != nil {
		return nil, fmt.Errorf("writing 80x80 icon: %w", err)
	}
	outputs = append(outputs, gif80Path)

	// Generate the 100x100 PNG from the 512x512 systray icon.
	srcPNG, err := os.ReadFile(filepath.Join(b.Repo, "client/systray/tailscale.png"))
	if err != nil {
		return nil, fmt.Errorf("reading systray icon: %w", err)
	}
	png100, err := scalePNG(srcPNG, 100)
	if err != nil {
		return nil, fmt.Errorf("scaling icon to 100x100: %w", err)
	}
	png100Path := filepath.Join(b.Out, "tailscale-icon-100.png")
	if err := os.WriteFile(png100Path, png100, 0644); err != nil {
		return nil, fmt.Errorf("writing 100x100 icon: %w", err)
	}
	outputs = append(outputs, png100Path)

	return outputs, nil
}
