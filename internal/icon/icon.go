package icon

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed sc-player.png
var png []byte

var Resource = fyne.NewStaticResource("sc-player.png", png)
