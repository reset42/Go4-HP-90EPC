package assets

import (
	"embed"
	"io/fs"
)

//go:embed ui/*
var embeddedUI embed.FS

func UI() fs.FS {
	sub, err := fs.Sub(embeddedUI, "ui")
	if err != nil {
		// fallback: liefert root (sollte aber nicht passieren)
		return embeddedUI
	}
	return sub
}

