package history

import (
	"net/url"
	"path/filepath"
)

func sqliteFileURI(absolutePath string, params url.Values) string {
	path := filepath.ToSlash(absolutePath)
	// A drive must be part of the URI path, not its authority: file:///C:/...
	// url.URL otherwise serializes C:/... as file://C:/..., which SQLite rejects.
	if len(path) >= 3 && path[1] == ':' && path[2] == '/' {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path, RawQuery: params.Encode()}).String()
}
