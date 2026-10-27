package format

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// DiffFormatter renders a lens.DiffResult.
type DiffFormatter struct {
	palette Palette
	summary bool
}

// DiffOption configures a DiffFormatter.
type DiffOption func(*DiffFormatter)

// WithDiffPalette sets the colour palette.
func WithDiffPalette(p Palette) DiffOption {
	return func(d *DiffFormatter) { d.palette = p }
}

// WithSummary enables ASCII summary table output.
func WithSummary(enabled bool) DiffOption {
	return func(d *DiffFormatter) { d.summary = enabled }
}

// NewDiffFormatter returns a DiffFormatter.
func NewDiffFormatter(opts ...DiffOption) *DiffFormatter {
	d := &DiffFormatter{palette: NoColor}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Format writes a readable rendering of the diff to w.
func (f *DiffFormatter) Format(w io.Writer, d *lens.DiffResult) error {
	if d == nil {
		return fmt.Errorf("format: nil diff")
	}
	p := f.palette
	var b strings.Builder

	if d.LeftType != d.RightType {
		fmt.Fprintf(&b, "%stype differs: %s vs %s%s\n\n", p.Change, d.LeftType, d.RightType, p.Reset)
	} else {
		fmt.Fprintf(&b, "%s%s%s\n\n", p.Type, d.LeftType, p.Reset)
	}

	if d.Equal() {
		fmt.Fprintf(&b, "%sno differences%s\n", p.Add, p.Reset)
		if len(d.Warnings) > 0 {
			fmt.Fprintf(&b, "\n")
			for _, warn := range d.Warnings {
				fmt.Fprintf(&b, "%swarning: %s%s\n", p.Change, warn, p.Reset)
			}
		}
		if _, err := io.WriteString(w, b.String()); err != nil {
			return fmt.Errorf("format: write diff: %w", err)
		}
		return nil
	}

	for _, c := range d.Changes {
		f.change(&b, c)
	}

	if len(d.Warnings) > 0 {
		fmt.Fprintf(&b, "\n")
		for _, warn := range d.Warnings {
			fmt.Fprintf(&b, "%swarning: %s%s\n", p.Change, warn, p.Reset)
		}
	}

	fmt.Fprintf(&b, "\n%s%s%s\n", p.Dim, d.Summary(), p.Reset)

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("format: write diff: %w", err)
	}
	return nil
}

func (f *DiffFormatter) change(b *strings.Builder, c lens.Change) {
	p := f.palette

	switch c.Op {
	case lens.ChangeAdded:
		fmt.Fprintf(b, "%s+ %s%s = %s\n", p.Add, c.Path, p.Reset, f.value(c.After, c.AfterNote))
	case lens.ChangeRemoved:
		fmt.Fprintf(b, "%s- %s%s = %s\n", p.Remove, c.Path, p.Reset, f.value(c.Before, c.BeforeNote))
	case lens.ChangeReordered:
		fmt.Fprintf(b, "%s~ %s%s: reordered from index %v to %v\n",
			p.Change, c.Path, p.Reset, c.Before, c.After)
	default:
		fmt.Fprintf(b, "%s~ %s%s\n", p.Change, c.Path, p.Reset)
		fmt.Fprintf(b, "    %s- %s%s\n", p.Remove, f.value(c.Before, c.BeforeNote), p.Reset)
		fmt.Fprintf(b, "    %s+ %s%s\n", p.Add, f.value(c.After, c.AfterNote), p.Reset)
	}
}

func (f *DiffFormatter) value(v any, note string) string {
	if v == nil && note == "" {
		return "(none)"
	}
	if v == nil {
		return note
	}
	s := fmt.Sprintf("%v", v)
	if note != "" {
		return s + " " + f.palette.Note + note + f.palette.Reset
	}
	return s
}

// DiffSummaryFormatter renders a lens.DiffResult as a concise ASCII summary table.
type DiffSummaryFormatter struct {
	palette Palette
}

// DiffSummaryOption configures a DiffSummaryFormatter.
type DiffSummaryOption func(*DiffSummaryFormatter)

// WithDiffSummaryPalette sets the colour palette.
func WithDiffSummaryPalette(p Palette) DiffSummaryOption {
	return func(d *DiffSummaryFormatter) { d.palette = p }
}

// NewDiffSummaryFormatter returns a DiffSummaryFormatter.
func NewDiffSummaryFormatter(opts ...DiffSummaryOption) *DiffSummaryFormatter {
	d := &DiffSummaryFormatter{palette: NoColor}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Format writes a summary table of the diff to w.
func (f *DiffSummaryFormatter) Format(w io.Writer, d *lens.DiffResult) error {
	if d == nil {
		return fmt.Errorf("format: nil diff")
	}
	p := f.palette
	var b strings.Builder

	if d.LeftType != d.RightType {
		fmt.Fprintf(&b, "%stype differs: %s vs %s%s\n\n", p.Change, d.LeftType, d.RightType, p.Reset)
	} else {
		fmt.Fprintf(&b, "%s%s%s\n\n", p.Type, d.LeftType, p.Reset)
	}

	if d.Equal() {
		fmt.Fprintf(&b, "%sno differences%s\n", p.Add, p.Reset)
		if len(d.Warnings) > 0 {
			fmt.Fprintf(&b, "\n")
			for _, warn := range d.Warnings {
				fmt.Fprintf(&b, "%swarning: %s%s\n", p.Change, warn, p.Reset)
			}
		}
		if _, err := io.WriteString(w, b.String()); err != nil {
			return fmt.Errorf("format: write diff summary: %w", err)
		}
		return nil
	}

	// Group changes by top-level section
	sectionCounts := make(map[string]map[lens.ChangeOp]int)
	for _, c := range d.Changes {
		section := topLevelSection(c.Path)
		if sectionCounts[section] == nil {
			sectionCounts[section] = make(map[lens.ChangeOp]int)
		}
		sectionCounts[section][c.Op]++
	}

	// Sort sections for deterministic output
	sections := make([]string, 0, len(sectionCounts))
	for s := range sectionCounts {
		sections = append(sections, s)
	}
	sort.Strings(sections)

	// Print table header
	fmt.Fprintf(&b, "%s%-30s %8s %8s %8s %8s%s\n", p.Type, "SECTION", "ADDED", "REMOVED", "MODIFIED", "REORDERED", p.Reset)
	fmt.Fprintf(&b, "%s%s%s\n", p.Dim, strings.Repeat("─", 70), p.Reset)

	for _, section := range sections {
		counts := sectionCounts[section]
		added := counts[lens.ChangeAdded]
		removed := counts[lens.ChangeRemoved]
		modified := counts[lens.ChangeModified]
		reordered := counts[lens.ChangeReordered]
		typeChanged := counts[lens.ChangeTypeChanged]
		// Combine type_changed into modified for display
		modified += typeChanged

		fmt.Fprintf(&b, "%-30s %8d %8d %8d %8d\n",
			truncate(section, 30), added, removed, modified, reordered)
	}

	fmt.Fprintf(&b, "%s%s%s\n", p.Dim, strings.Repeat("─", 70), p.Reset)

	// Totals
	totalAdded, totalRemoved, totalModified, totalReordered := 0, 0, 0, 0
	for _, counts := range sectionCounts {
		totalAdded += counts[lens.ChangeAdded]
		totalRemoved += counts[lens.ChangeRemoved]
		totalModified += counts[lens.ChangeModified] + counts[lens.ChangeTypeChanged]
		totalReordered += counts[lens.ChangeReordered]
	}
	fmt.Fprintf(&b, "%-30s %8d %8d %8d %8d\n", "TOTAL", totalAdded, totalRemoved, totalModified, totalReordered)

	if len(d.Warnings) > 0 {
		fmt.Fprintf(&b, "\n")
		for _, warn := range d.Warnings {
			fmt.Fprintf(&b, "%swarning: %s%s\n", p.Change, warn, p.Reset)
		}
	}

	fmt.Fprintf(&b, "\n%s%s%s\n", p.Dim, d.Summary(), p.Reset)

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("format: write diff summary: %w", err)
	}
	return nil
}

// topLevelSection extracts the top-level section from a path.
func topLevelSection(path string) string {
	if path == "" {
		return "(root)"
	}
	// Split by dot, but handle bracketed indices
	segments := strings.Split(path, ".")
	if len(segments) == 0 {
		return "(root)"
	}
	first := segments[0]
	// If first segment contains [, it's an array at root
	if strings.Contains(first, "[") {
		idx := strings.Index(first, "[")
		return first[:idx]
	}
	return first
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// DiffChangeDoc represents a single difference in the structured JSON diff schema.
type DiffChangeDoc struct {
	Path       string `json:"path"`
	Op         string `json:"op"`
	Before     any    `json:"before,omitempty"`
	After      any    `json:"after,omitempty"`
	BeforeNote string `json:"before_note,omitempty"`
	AfterNote  string `json:"after_note,omitempty"`
}

// DiffDocument represents the full structured diff in the stable JSON schema.
type DiffDocument struct {
	LeftType  string          `json:"left_type"`
	RightType string          `json:"right_type"`
	Equal     bool            `json:"equal"`
	Summary   string          `json:"summary"`
	Changes   []DiffChangeDoc `json:"changes"`
	Redacted  []string        `json:"redacted,omitempty"`
	Warnings  []string        `json:"warnings,omitempty"`
}

// BuildDiffDocument converts a lens.DiffResult into the stable JSON DiffDocument.
func BuildDiffDocument(d *lens.DiffResult) DiffDocument {
	if d == nil {
		return DiffDocument{}
	}
	changes := make([]DiffChangeDoc, 0, len(d.Changes))
	for _, c := range d.Changes {
		changes = append(changes, DiffChangeDoc{
			Path:       c.Path,
			Op:         c.Op.String(),
			Before:     c.Before,
			After:      c.After,
			BeforeNote: c.BeforeNote,
			AfterNote:  c.AfterNote,
		})
	}
	return DiffDocument{
		LeftType:  d.LeftType,
		RightType: d.RightType,
		Equal:     d.Equal(),
		Summary:   d.Summary(),
		Changes:   changes,
		Redacted:  d.Redacted,
		Warnings:  d.Warnings,
	}
}

// DiffJSONFormatter renders a lens.DiffResult as structured JSON adhering to the stable schema.
type DiffJSONFormatter struct {
	indent string
}

// DiffJSONOption configures a DiffJSONFormatter.
type DiffJSONOption func(*DiffJSONFormatter)

// WithDiffJSONIndent configures indentation for DiffJSONFormatter.
func WithDiffJSONIndent(indent string) DiffJSONOption {
	return func(f *DiffJSONFormatter) { f.indent = indent }
}

// NewDiffJSONFormatter returns a new DiffJSONFormatter.
func NewDiffJSONFormatter(opts ...DiffJSONOption) *DiffJSONFormatter {
	f := &DiffJSONFormatter{indent: "  "}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Format writes structured JSON representation of the diff result to w.
func (f *DiffJSONFormatter) Format(w io.Writer, d *lens.DiffResult) error {
	if d == nil {
		return fmt.Errorf("format: nil diff")
	}
	doc := BuildDiffDocument(d)
	enc := json.NewEncoder(w)
	if f.indent != "" {
		enc.SetIndent("", f.indent)
	}
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("format: encoding json diff: %w", err)
	}
	return nil
}
