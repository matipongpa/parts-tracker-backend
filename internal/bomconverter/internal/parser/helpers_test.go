package parser

import (
	"io"
	"strings"
)

func bytesReader(s string) io.Reader { return strings.NewReader(s) }
