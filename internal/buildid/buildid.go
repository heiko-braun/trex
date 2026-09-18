// Package buildid computes the content-addressed Build ID for a workflow
// definition, per the versioning model in
// docs/architecure/zigflow-workflow-server-architecture.md: a hash of the
// normalized YAML plus the pinned Zigflow module version, so any change to
// either the definition or the Zigflow release in use produces a new,
// distinct Build ID.
package buildid

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime/debug"
)

const zigflowModulePath = "github.com/zigflow/zigflow"

// Compute returns the first 12 hex characters of SHA-256(yamlBytes +
// zigflowVersion).
func Compute(yamlBytes []byte) (string, error) {
	v, err := zigflowVersion()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(yamlBytes)
	h.Write([]byte(v))
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

// zigflowVersion returns the resolved version of the github.com/zigflow/zigflow
// module this binary was built against, read from the embedded build info
// so it can never drift from what go.mod actually pins.
func zigflowVersion() (string, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", fmt.Errorf("buildid: no build info available")
	}
	for _, dep := range info.Deps {
		if dep.Path == zigflowModulePath {
			return dep.Version, nil
		}
	}
	return "", fmt.Errorf("buildid: module %s not found in build info", zigflowModulePath)
}
