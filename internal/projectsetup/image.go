package projectsetup

import (
	"fmt"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/setup"
)

// The sandbox image: jailwick's base (or the default set in config.yaml), a base image of the
// user's, a Dockerfile in the repo, or an image used as is. On the first three jailwick builds
// its layer (its tools, the stacks, apt packages, Playwright, Claude Code, the user's home); the
// last one gets nothing added. A base must be Debian or Ubuntu and have the sandbox user.

// Image sources: the values of image.source.
const (
	SourceDefault    = "default"
	SourceBase       = "base"
	SourceDockerfile = "dockerfile"
	SourceAsIs       = "image"
)

func imageFields() []Field {
	return []Field{
		{
			Key: "image.source", Flag: "image", Label: "Sandbox image", Kind: setup.Choice,
			Help: "What the sandbox is built on. Stacks, apt packages and Playwright are added on top of the first three.",
			OptionsOf: func(*Draft) []Option {
				return []Option{
					{Value: SourceDefault, Label: "The default", Help: "jailwick's base (Debian, Node and the usual tools), or the default image set in Settings"},
					{Value: SourceBase, Label: "Your base image", Help: "a Debian or Ubuntu image, like eclipse-temurin:21-jdk or ruby:3.3-bookworm"},
					{Value: SourceDockerfile, Label: "Your Dockerfile", Help: "a Dockerfile in the repo, built first"},
					{Value: SourceAsIs, Label: "An image as is", Tag: "advanced", Help: "one your team builds and keeps up",
						Warn: "nothing is added: it must already have Claude Code, git, the sandbox user and its home"},
				}
			},
			NoteOf: languageNote,
			Get:    func(d *Draft) any { return d.imageSource() },
			Set: func(d *Draft, v any) error {
				d.ImageSource = v.(string)
				img := &d.Project.Image // only the chosen source's value stays
				if d.ImageSource == SourceBase && img.Base == "" {
					img.Base = suggestedBase(d) // what the repo's language needs, to start from
				}
				if d.ImageSource != SourceBase {
					img.Base = ""
				}
				if d.ImageSource != SourceDockerfile {
					img.Dockerfile, img.Context = "", ""
				}
				if d.ImageSource != SourceAsIs {
					img.Name = ""
				}
				return nil
			},
		},
		text("image.base", "base", "Base image", "Pulled from its registry. Check tries it: Debian or Ubuntu, the user, the architecture.",
			func(p *config.Project) *string { return &p.Image.Base }).
			WithPlaceholder("eclipse-temurin:21-jdk").CheckedBy(config.CheckImageRef).WithAction("check-image").When("image.source", SourceBase),
		text("image.dockerfile", "dockerfile", "Dockerfile", "A path in the repo; its folder is the build context. Its image must be Debian or Ubuntu.",
			func(p *config.Project) *string { return &p.Image.Dockerfile }).
			WithPlaceholder("docker/dev.Dockerfile").When("image.source", SourceDockerfile),
		text("image.name", "image-name", "Image", "Used as is. Check tries it: Claude Code, git and the user must be in it.",
			func(p *config.Project) *string { return &p.Image.Name }).
			WithPlaceholder("ghcr.io/acme/dev-sandbox:2").CheckedBy(config.CheckImageRef).WithAction("check-image").When("image.source", SourceAsIs),
		text("image.user", "image-user", "Sandbox user", "The user sessions run as; the image must have it. Empty: node.",
			func(p *config.Project) *string { return &p.Image.User }).
			WithPlaceholder("node").CheckedBy(config.CheckUserName).Unless("image.source", SourceDefault),
		text("image.home", "image-home", "Home folder", "That user's home. Empty: /home/node.",
			func(p *config.Project) *string { return &p.Image.Home }).
			WithPlaceholder("/home/node").CheckedBy(config.CheckHome).Unless("image.source", SourceDefault),
	}
}

// imageSource is the image answer: as given, else what the project has.
func (d *Draft) imageSource() string {
	if d.ImageSource != "" {
		return d.ImageSource
	}
	switch img := d.Project.Image; {
	case img.Name != "":
		return SourceAsIs
	case img.Dockerfile != "":
		return SourceDockerfile
	case img.Base != "":
		return SourceBase
	}
	return SourceDefault
}

// checkImage requires the chosen source's value, and a Dockerfile in the repo.
func (d *Draft) checkImage() error {
	img := d.Project.Image
	switch d.imageSource() {
	case SourceBase:
		if img.Base == "" {
			return problem("Base image: name the image, like eclipse-temurin:21-jdk")
		}
	case SourceDockerfile:
		if img.Dockerfile == "" {
			return problem("Dockerfile: give its path in the repo")
		}
	case SourceAsIs:
		if img.Name == "" {
			return problem("Image: name the image to use as is")
		}
	}
	return nil
}

// languageNote says which languages the repo uses that no stack installs, and how to get them:
// Debian's packages on the default base, or a base image that has them.
func languageNote(d *Draft) string {
	if d == nil || len(d.Detected.Languages) == 0 || d.imageSource() == SourceAsIs || d.imageSource() == SourceDockerfile {
		return ""
	}
	var notes []string
	for _, l := range d.Detected.Languages {
		n := fmt.Sprintf("The repo uses %s (%s), which no stack installs:", l.Name, l.File)
		switch {
		case len(l.Apt) > 0 && l.Base != "":
			n += fmt.Sprintf(" add the apt packages %s (Toolset), or use a base image like %s.", strings.Join(l.Apt, ", "), l.Base)
		case len(l.Apt) > 0:
			n += fmt.Sprintf(" add the apt packages %s (Toolset).", strings.Join(l.Apt, ", "))
		default:
			n += fmt.Sprintf(" use a base image like %s.", l.Base)
		}
		notes = append(notes, n)
	}
	if suggestedBase(d) != "" {
		notes = append(notes, "Check image tells whether a base image has the sandbox user; docs/IMAGES.md shows how to add one.")
	}
	return strings.Join(notes, " ")
}

// suggestedBase is the base image of the first language the repo uses that no stack installs.
func suggestedBase(d *Draft) string {
	for _, l := range d.Detected.Languages {
		if l.Base != "" {
			return l.Base
		}
	}
	return ""
}
