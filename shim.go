// Package verifierkit carries the files the Go spine takes from tools/, so that bin/spine needs
// no checkout beside it.
package verifierkit

import _ "embed"

// AdapterShim is tools/adapter_shim.py, which conform --adapter runs with Python to serve a
// Python adapter over the adapter protocol.
//
//go:embed tools/adapter_shim.py
var AdapterShim []byte
