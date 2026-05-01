package mdconv

type ImageMode int

const (
	ImagesEmbed ImageMode = iota // inline as data: URIs
	ImagesDrop                   // strip entirely
)

type Options struct {
	Images ImageMode
	Marp   bool
}
