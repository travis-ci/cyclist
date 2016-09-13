package cyclist

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"
)

var (
	// VersionString is a version!
	VersionString = "?"
	// RevisionString is a revision!
	RevisionString = "?"
	// RevisionURLString is a revision URL!
	RevisionURLString = "?"
	// GeneratedString is a timestamp!
	GeneratedString = "?"
	// CopyrightString is legalese!
	CopyrightString = "?"

	// RedisNamespace is the namespace used in redis OK!
	RedisNamespace = "cyclist"
)

func init() {
	cli.VersionPrinter = customVersionPrinter
}

func customVersionPrinter(ctx *cli.Context) {
	fmt.Fprintf(ctx.App.Writer, "%s v=%s rev=%s d=%s\n",
		ctx.App.Name, VersionString, RevisionString, GeneratedString)
}
