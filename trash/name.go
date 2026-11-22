// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package trash

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// freeName is a path in dir for a file named name that nothing uses yet:
// name, else name with " 2", " 3"… before its extension
func freeName(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	p := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return p
		}
		p = filepath.Join(dir, fmt.Sprintf("%s %d%s", stem, i, ext))
	}
}
