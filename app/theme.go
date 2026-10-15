package app

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type angularTheme struct {
	fyne.Theme
	dark bool
}

func newAngularTheme(dark bool) fyne.Theme {
	return &angularTheme{Theme: theme.DefaultTheme(), dark: dark}
}

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: 255} }

func (t *angularTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	variant := theme.VariantLight
	if t.dark {
		variant = theme.VariantDark
	}
	switch name {
	case theme.ColorNamePrimary, theme.ColorNameHyperlink, theme.ColorNameFocus:
		if t.dark {
			return rgb(255, 112, 64)
		}
		return rgb(196, 62, 16)
	case theme.ColorNameForegroundOnPrimary:
		return rgb(255, 255, 255)
	}
	if t.dark {
		switch name {
		case theme.ColorNameBackground:
			return rgb(20, 22, 25)
		case theme.ColorNameInputBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
			return rgb(30, 33, 37)
		case theme.ColorNameButton:
			return rgb(38, 41, 46)
		case theme.ColorNameSeparator, theme.ColorNameInputBorder:
			return rgb(58, 62, 68)
		}
	} else {
		switch name {
		case theme.ColorNameBackground:
			return rgb(244, 244, 242)
		case theme.ColorNameInputBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
			return rgb(255, 255, 255)
		case theme.ColorNameButton:
			return rgb(228, 228, 225)
		case theme.ColorNameSeparator, theme.ColorNameInputBorder:
			return rgb(200, 200, 196)
		}
	}
	return t.Theme.Color(name, variant)
}

func (t *angularTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameButtonRadius, theme.SizeNameInputRadius, theme.SizeNameCardRadius,
		theme.SizeNameSelectionRadius, theme.SizeNameDialogRadius, theme.SizeNamePopupRadius,
		theme.SizeNameMenuRadius, theme.SizeNameScrollBarRadius, theme.SizeNameWindowButtonRadius,
		theme.SizeNameInnerWindowRadius:
		return 0
	case theme.SizeNameInputBorder, theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNamePadding:
		return 6
	}
	return t.Theme.Size(name)
}

func (a *Application) primary() color.Color {
	return newAngularTheme(a.dark).Color(theme.ColorNamePrimary, theme.VariantDark)
}

func (a *Application) accent(width, height float32) *canvas.Rectangle {
	block := canvas.NewRectangle(a.primary())
	block.SetMinSize(fyne.NewSize(width, height))
	a.accents = append(a.accents, block)
	return block
}

func (a *Application) paintAccents() {
	for _, block := range a.accents {
		block.FillColor = a.primary()
		block.Refresh()
	}
}

func (a *Application) Heading(text string) fyne.CanvasObject {
	label := widget.NewLabelWithStyle(strings.ToUpper(text), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewBorder(nil, nil, a.accent(4, 18), nil, label)
}

func framed(content fyne.CanvasObject) fyne.CanvasObject {
	frame := canvas.NewRectangle(color.Transparent)
	frame.StrokeColor = color.NRGBA{R: 128, G: 128, B: 128, A: 110}
	frame.StrokeWidth = 1
	return container.NewStack(frame, container.NewPadded(content))
}
