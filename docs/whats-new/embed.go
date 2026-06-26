package whatsnew

import "embed"

// Files contains English release notes committed before each tagged release.
//
//go:embed v*.md
var Files embed.FS
