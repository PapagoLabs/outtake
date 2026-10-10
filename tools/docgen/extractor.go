// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"cmp"
	"iter"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// cobraExtractor builds documentation data from cobra command metadata.
type cobraExtractor struct {
	// urlBase is the site path of the CLI reference, with slashes on both ends.
	urlBase string
}

const (
	// maxDescriptionLen is the longest page description, in runes, including
	// the ellipsis that marks a truncated one.
	maxDescriptionLen = 160
	// ellipsis marks a truncated description.
	ellipsis = "..."
	// indexTitle is the CLI reference index page title.
	indexTitle = "CLI Reference"
	// referenceURL is the site path of the CLI reference.
	referenceURL = "/cli-reference/"
	// paragraphBreak separates paragraphs in help and example text.
	paragraphBreak = "\n\n"
	// commentPrefix starts an example's title line.
	commentPrefix = "#"
	// indentChars are the characters dedent removes.
	indentChars = " \t"
	// wordSeparator joins the words of a path, a title, or folded text.
	wordSeparator = " "
)

// NewCobraExtractor returns an extractor that links pages under the CLI
// reference section.
//
// Returns:
//   - *cobraExtractor: the extractor.
func NewCobraExtractor() *cobraExtractor {
	return &cobraExtractor{urlBase: referenceURL}
}

// Extract builds the documentation for cmd and every available descendant.
//
// Parameters:
//   - cmd: the command to document.
//   - parentPath: the parent's full path, or cmd's own name for the root.
//
// Returns:
//   - *commandDoc: the documentation data.
func (extractor *cobraExtractor) Extract(cmd *cobra.Command, parentPath string) *commandDoc {
	fullPath := extractor.buildFullPath(cmd, parentPath)

	doc := &commandDoc{
		Name:        cmd.Name(),
		Short:       cmd.Short,
		Long:        strings.TrimSpace(cmd.Long),
		Use:         cmd.Use,
		Example:     cmd.Example,
		UseLine:     extractor.buildUseLine(cmd.Use, fullPath),
		Title:       titleCase(cmd.Name()),
		Description: extractor.buildDescription(cmp.Or(cmd.Long, cmd.Short)),
		FullPath:    fullPath,
		Flags:       nil,
		Inherited:   nil,
		Examples:    nil,
		SubCommands: nil,
		HasSubs:     false,
		Index:       nil,
	}

	if cmd.Example != "" {
		doc.Examples = extractor.parseExamples(cmd.Example, fullPath, cmd.Use)
	}

	if own := cmd.NonInheritedFlags(); own.HasAvailableFlags() {
		doc.Flags = extractor.buildFlags(own)
	}

	if cmd.HasAvailableInheritedFlags() {
		doc.Inherited = extractor.buildFlags(cmd.InheritedFlags())
	}

	if cmd.HasAvailableSubCommands() {
		doc.HasSubs = true
		doc.Index = extractor.buildIndex(cmd)
	}

	for child := range availableCommands(cmd) {
		doc.SubCommands = append(doc.SubCommands, extractor.Extract(child, fullPath))
	}

	return doc
}

// buildDescription turns long help text into a one-line page description.
//
// The first paragraph is used, with newlines folded into spaces so it fits a
// frontmatter line. A longer text is cut on a rune boundary and ends with an
// ellipsis.
//
// Parameters:
//   - long: the long help text.
//
// Returns:
//   - string: at most maxDescriptionLen runes.
func (*cobraExtractor) buildDescription(long string) string {
	desc := firstParagraph(long)
	if utf8.RuneCountInString(desc) <= maxDescriptionLen {
		return desc
	}

	runes := []rune(desc)
	cut := strings.TrimRightFunc(string(runes[:maxDescriptionLen-len(ellipsis)]), unicode.IsSpace)

	return cut + ellipsis
}

// buildFlags lists the visible flags of a flag set in name order.
//
// Parameters:
//   - flags: the flag set.
//
// Returns:
//   - []flagDoc: one entry per flag that is neither hidden nor deprecated.
func (*cobraExtractor) buildFlags(flags *pflag.FlagSet) []flagDoc {
	var docs []flagDoc

	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden || flag.Deprecated != "" {
			return
		}

		docs = append(docs, flagDoc{
			Name:      flag.Name,
			Shorthand: flag.Shorthand,
			Default:   flag.DefValue,
			Type:      flag.Value.Type(),
			Usage:     flag.Usage,
		})
	})

	return docs
}

// buildFullPath joins the parent path and the command name.
//
// Parameters:
//   - cmd: the command.
//   - parentPath: the parent's full path, or cmd's own name for the root.
//
// Returns:
//   - string: the full path, such as "outtake server start".
func (*cobraExtractor) buildFullPath(cmd *cobra.Command, parentPath string) string {
	if cmd.Name() == parentPath {
		return parentPath
	}

	return parentPath + wordSeparator + cmd.Name()
}

// buildIndex builds the index page for a command with child commands.
//
// Parameters:
//   - cmd: the command whose children the index lists.
//
// Returns:
//   - *indexDoc: the index data.
func (extractor *cobraExtractor) buildIndex(cmd *cobra.Command) *indexDoc {
	index := &indexDoc{
		Title:       indexTitle,
		Description: "Every " + cmd.Name() + " command and flag, generated from the program itself",
		Name:        cmd.Name(),
		Long:        strings.TrimSpace(cmd.Long),
		Sections:    nil,
		Examples:    nil,
		Flags:       nil,
	}

	if cmd.Example != "" {
		index.Examples = extractor.parseExamples(cmd.Example, cmd.CommandPath(), cmd.Use)
	}

	if own := cmd.NonInheritedFlags(); own.HasAvailableFlags() {
		index.Flags = extractor.buildFlags(own)
	}

	for child := range availableCommands(cmd) {
		index.Sections = append(index.Sections, extractor.buildSection(child))
	}

	return index
}

// buildSection builds the index entry for one top-level command.
//
// A group lists its children, and a leaf command lists itself.
//
// Parameters:
//   - child: the top-level command.
//
// Returns:
//   - sectionEntry: the entry.
func (extractor *cobraExtractor) buildSection(child *cobra.Command) sectionEntry {
	section := sectionEntry{
		Name:        child.Name(),
		Description: firstParagraph(child.Long),
		HasSubs:     child.HasAvailableSubCommands(),
		Title:       titleCase(child.Name()),
		SubCommands: nil,
	}

	if !section.HasSubs {
		section.SubCommands = []subCommandEntry{{
			Name:        child.Name(),
			Description: child.Short,
			URL:         extractor.urlBase + child.Name() + "/",
		}}

		return section
	}

	for sub := range availableCommands(child) {
		section.SubCommands = append(section.SubCommands, subCommandEntry{
			Name:        child.Name() + wordSeparator + sub.Name(),
			Description: sub.Short,
			URL:         extractor.urlBase + child.Name() + "/" + sub.Name() + "/",
		})
	}

	return section
}

// buildUseLine replaces the command name in a Use line with the full path.
//
// Parameters:
//   - use: the cobra Use line.
//   - fullPath: the command's full path.
//
// Returns:
//   - string: the usage line, or empty for an empty Use line.
func (*cobraExtractor) buildUseLine(use, fullPath string) string {
	if strings.TrimSpace(use) == "" {
		return ""
	}

	args := useArgs(use)
	if args == "" {
		return fullPath
	}

	return fullPath + wordSeparator + args
}

// parseExamples splits cobra Example text into titled blocks.
//
// Blocks are separated by blank lines. Lines starting with "#" form the
// block's title and the remaining lines its code. An example that only
// repeats the usage line is dropped, because the page already shows it.
//
// Parameters:
//   - raw: the cobra Example text.
//   - fullPath: the command's full path.
//   - use: the cobra Use line.
//
// Returns:
//   - []exampleDoc: the examples, or nil when none add anything.
func (extractor *cobraExtractor) parseExamples(raw, fullPath, use string) []exampleDoc {
	var examples []exampleDoc

	for para := range strings.SplitSeq(strings.TrimSpace(raw), paragraphBreak) {
		example, ok := parseExample(para)
		if ok {
			examples = append(examples, example)
		}
	}

	if len(examples) == 1 && examples[0].Code == extractor.buildUseLine(use, fullPath) {
		return nil
	}

	return examples
}

// parseExample splits one example block into its title and code.
//
// Parameters:
//   - para: the block, without the blank lines around it.
//
// Returns:
//   - exampleDoc: the example.
//   - bool: false when the block holds no code.
func parseExample(para string) (exampleDoc, bool) {
	var (
		titles    []string
		codeLines []string
	)

	for line := range strings.SplitSeq(para, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
		case strings.HasPrefix(trimmed, commentPrefix):
			titles = append(titles, strings.TrimSpace(strings.TrimLeft(trimmed, commentPrefix)))
		default:
			codeLines = append(codeLines, strings.TrimRightFunc(line, unicode.IsSpace))
		}
	}

	if len(codeLines) == 0 {
		return exampleDoc{Title: "", Code: ""}, false
	}

	return exampleDoc{
		Title: strings.TrimSpace(strings.Join(titles, wordSeparator)),
		Code:  dedent(codeLines),
	}, true
}

// availableCommands yields the children of cmd that a user can run.
//
// Hidden, deprecated, and help topic commands are skipped. Cobra returns
// children sorted by name, so the order is stable.
//
// Parameters:
//   - cmd: the parent command.
//
// Returns:
//   - [iter.Seq]: the available children.
func availableCommands(cmd *cobra.Command) iter.Seq[*cobra.Command] {
	return slices.Values(slices.DeleteFunc(slices.Clone(cmd.Commands()), undocumented))
}

// undocumented reports whether a command stays out of the reference.
//
// Parameters:
//   - cmd: the command.
//
// Returns:
//   - bool: true for a hidden, deprecated, or help topic command.
func undocumented(cmd *cobra.Command) bool {
	return !cmd.IsAvailableCommand() || cmd.IsAdditionalHelpTopicCommand()
}

// dedent removes the indentation every line shares.
//
// Only spaces and tabs count as indentation, so a cut never splits a rune.
//
// Parameters:
//   - lines: lines with trailing space removed. At least one is not blank.
//
// Returns:
//   - string: the lines joined by newlines.
func dedent(lines []string) string {
	indent := -1

	for _, line := range lines {
		width := len(line) - len(strings.TrimLeft(line, indentChars))
		if indent < 0 || width < indent {
			indent = width
		}
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, line[indent:])
	}

	return strings.Join(out, "\n")
}

// firstParagraph returns the first paragraph of text on one line.
//
// Parameters:
//   - text: the text.
//
// Returns:
//   - string: the first paragraph with runs of white space folded into
//     single spaces.
func firstParagraph(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.Index(text, paragraphBreak); idx != -1 {
		text = text[:idx]
	}

	return strings.Join(strings.Fields(text), wordSeparator)
}

// titleCase upper-cases the first letter of each space-separated word.
//
// Parameters:
//   - text: the text, such as "notify-test".
//
// Returns:
//   - string: the title, such as "Notify-test".
func titleCase(text string) string {
	words := strings.Fields(text)
	for i, word := range words {
		first, size := utf8.DecodeRuneInString(word)

		words[i] = string(unicode.ToTitle(first)) + word[size:]
	}

	return strings.Join(words, wordSeparator)
}

// useArgs returns the argument part of a cobra Use line.
//
// Parameters:
//   - use: the cobra Use line, such as "status [flags]".
//
// Returns:
//   - string: everything after the command name, trimmed.
func useArgs(use string) string {
	use = strings.TrimSpace(use)

	_, args, _ := strings.Cut(use, wordSeparator)

	return strings.TrimSpace(args)
}
