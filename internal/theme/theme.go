package theme

import "strings"

type Theme struct {
	Name            string
	Background      string
	Title           string
	PaletteBorder   string
	PromptBorder    string
	Header          string
	Muted           string
	Prompt          string
	Query           string
	SearchBG        string
	SearchFG        string
	Description     string
	Empty           string
	Chip            string
	ChipBG          string
	SelectedChip    string
	SelectedChipBG  string
	Glyph           string
	MatchFG         string
	SelectedMatchFG string
	SelectedFG      string
	SelectedBG      string
}

func Default() Theme {
	return Resolve("shades-of-purple")
}

func Resolve(name string) Theme {
	switch normalize(name) {
	case "catppuccin":
		return Theme{
			Name:            "catppuccin",
			Background:      "#1e1e2e",
			Title:           "#cdd6f4",
			PaletteBorder:   "#cdd6f4",
			PromptBorder:    "#cdd6f4",
			Header:          "#f9e2af",
			Muted:           "#6c7086",
			Prompt:          "#f9e2af",
			Query:           "#cdd6f4",
			SearchBG:        "#45475a",
			SearchFG:        "#f5f7ff",
			Description:     "#6f748a",
			Empty:           "#6c7086",
			Chip:            "#cdd6f4",
			ChipBG:          "#313244",
			SelectedChip:    "#f9e2af",
			SelectedChipBG:  "#313244",
			Glyph:           "#f9e2af",
			MatchFG:         "#f9e2af",
			SelectedMatchFG: "#f9e2af",
			SelectedFG:      "#f5f7ff",
			SelectedBG:      "#45475a",
		}
	case "tokyonight":
		return Theme{
			Name:            "tokyonight",
			Background:      "#1a1b26",
			Title:           "#c0caf5",
			PaletteBorder:   "#c0caf5",
			PromptBorder:    "#c0caf5",
			Header:          "#e0af68",
			Muted:           "#565f89",
			Prompt:          "#e0af68",
			Query:           "#c0caf5",
			SearchBG:        "#292e42",
			SearchFG:        "#ffffff",
			Description:     "#5f668c",
			Empty:           "#565f89",
			Chip:            "#c0caf5",
			ChipBG:          "#24283b",
			SelectedChip:    "#e0af68",
			SelectedChipBG:  "#24283b",
			Glyph:           "#e0af68",
			MatchFG:         "#e0af68",
			SelectedMatchFG: "#e0af68",
			SelectedFG:      "#ffffff",
			SelectedBG:      "#292e42",
		}
	case "rosepine":
		return Theme{
			Name:            "rosepine",
			Background:      "#191724",
			Title:           "#e0def4",
			PaletteBorder:   "#e0def4",
			PromptBorder:    "#e0def4",
			Header:          "#f6c177",
			Muted:           "#6e6a86",
			Prompt:          "#f6c177",
			Query:           "#e0def4",
			SearchBG:        "#403d52",
			SearchFG:        "#ffffff",
			Description:     "#6f6a86",
			Empty:           "#6e6a86",
			Chip:            "#e0def4",
			ChipBG:          "#26233a",
			SelectedChip:    "#f6c177",
			SelectedChipBG:  "#26233a",
			Glyph:           "#f6c177",
			MatchFG:         "#f6c177",
			SelectedMatchFG: "#f6c177",
			SelectedFG:      "#ffffff",
			SelectedBG:      "#403d52",
		}
	case "kanagawa":
		return Theme{
			Name:            "kanagawa",
			Background:      "#1f1f28",
			Title:           "#dcd7ba",
			PaletteBorder:   "#dcd7ba",
			PromptBorder:    "#dcd7ba",
			Header:          "#e6c384",
			Muted:           "#727169",
			Prompt:          "#e6c384",
			Query:           "#dcd7ba",
			SearchBG:        "#363646",
			SearchFG:        "#ffffff",
			Description:     "#7f7a72",
			Empty:           "#727169",
			Chip:            "#dcd7ba",
			ChipBG:          "#2a2a37",
			SelectedChip:    "#e6c384",
			SelectedChipBG:  "#2a2a37",
			Glyph:           "#e6c384",
			MatchFG:         "#e6c384",
			SelectedMatchFG: "#e6c384",
			SelectedFG:      "#ffffff",
			SelectedBG:      "#363646",
		}
	case "", "shades-of-purple":
		return Theme{
			Name:            "shades-of-purple",
			Background:      "#2d2b55",
			Title:           "#d7d3ff",
			PaletteBorder:   "#d7d3ff",
			PromptBorder:    "#d7d3ff",
			Header:          "#fad000",
			Muted:           "#a599e9",
			Prompt:          "#fad000",
			Query:           "#ffffff",
			SearchBG:        "#403b75",
			SearchFG:        "#ffffff",
			Description:     "#7d75ad",
			Empty:           "#a599e9",
			Chip:            "#d7d3ff",
			ChipBG:          "#1e1e3f",
			SelectedChip:    "#fad000",
			SelectedChipBG:  "#1e1e3f",
			Glyph:           "#fad000",
			MatchFG:         "#fad000",
			SelectedMatchFG: "#fad000",
			SelectedFG:      "#ffffff",
			SelectedBG:      "#403b75",
		}
	case "solarized":
		return Theme{
			Name:            "solarized",
			Background:      "#002b36",
			Title:           "#eee8d5",
			PaletteBorder:   "#eee8d5",
			PromptBorder:    "#eee8d5",
			Header:          "#b58900",
			Muted:           "#586e75",
			Prompt:          "#b58900",
			Query:           "#fdf6e3",
			SearchBG:        "#164955",
			SearchFG:        "#fdf6e3",
			Description:     "#586e75",
			Empty:           "#586e75",
			Chip:            "#eee8d5",
			ChipBG:          "#073642",
			SelectedChip:    "#b58900",
			SelectedChipBG:  "#073642",
			Glyph:           "#b58900",
			MatchFG:         "#b58900",
			SelectedMatchFG: "#b58900",
			SelectedFG:      "#fdf6e3",
			SelectedBG:      "#164955",
		}
	case "gruvbox":
		return Theme{
			Name:            "gruvbox",
			Background:      "#282828",
			Title:           "#ebdbb2",
			PaletteBorder:   "#ebdbb2",
			PromptBorder:    "#ebdbb2",
			Header:          "#fabd2f",
			Muted:           "#928374",
			Prompt:          "#fabd2f",
			Query:           "#fbf1c7",
			SearchBG:        "#504945",
			SearchFG:        "#fbf1c7",
			Description:     "#928374",
			Empty:           "#928374",
			Chip:            "#ebdbb2",
			ChipBG:          "#3c3836",
			SelectedChip:    "#fabd2f",
			SelectedChipBG:  "#3c3836",
			Glyph:           "#fabd2f",
			MatchFG:         "#fabd2f",
			SelectedMatchFG: "#fabd2f",
			SelectedFG:      "#fbf1c7",
			SelectedBG:      "#504945",
		}
	default:
		return Default()
	}
}

func Names() []string {
	return []string{
		"catppuccin",
		"tokyonight",
		"rosepine",
		"kanagawa",
		"shades-of-purple",
		"solarized",
		"gruvbox",
	}
}

func normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
