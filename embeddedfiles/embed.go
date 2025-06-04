package embeddedfiles

import "embed"

//go:embed image_template/*
var ImageTemplate embed.FS

const ImageTemplateBasePath = "image_template"

//go:embed v2_image_template/*
var V2ImageTemplate embed.FS

const V2ImageTemplateBasePath = "v2_image_template"
