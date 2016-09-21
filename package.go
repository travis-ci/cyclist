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

	cyclistMetadata = struct {
		Version     string `json:"version"`
		Revision    string `json:"revision"`
		RevisionURL string `json:"revision_url"`
		Generated   string `json:"generated"`
	}{}
)

func init() {
	cli.VersionPrinter = customVersionPrinter

	cyclistMetadata.Version = VersionString
	cyclistMetadata.Revision = RevisionString
	cyclistMetadata.RevisionURL = RevisionURLString
	cyclistMetadata.Generated = GeneratedString
}

func customVersionPrinter(ctx *cli.Context) {
	fmt.Fprintf(ctx.App.Writer, "%s v=%s rev=%s d=%s\n",
		ctx.App.Name, VersionString, RevisionString, GeneratedString)
}
