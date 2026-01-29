# Built-in layers

This directory contains a selection of base layers from which users can choose.
Every bundle will have one of these base layers.

In each release of the cli, one of these base layers is default.
Users can specify a different layer through a flag.

## Adding a layer

To add a layer:

1. Create a new layer in a subdirectory (`avdcli layer new -o embeddedfiles/v2_default_layers/<layer_name>`).
2. Add an `embed` statement in the `embed.go` file. This will make sure the folder is embedded during the build process.
3. Add a name `const` with the name of this layer.
4. Add an entry to the base layer map in `embed.go`.

## Changing the default

1. Beta phase
   1. Add a new base layer to the project (just a new layer in this directory).
   2. Configure a `base_image` in the `properties.json5` of that layer.
   3. Register the new base layer in `embed.go` with a `-beta` suffix in the name.
   4. Release the cli and inform avd admins that they can start testing with the new base image.
   5. Gather feedback. During iteration, you can update this layer folder.
2. Availability phase
   1. Duplicate the layer folder and the registration in `embed.go` but without the `-beta` suffix in the name.
   2. Release the cli and Inform avd admins about the general availability of the layer.
   3. Gather feedback.
3. Default phase
   1. Add a warning to the current default to notify admins about the upcoming change in default. Mention the flag to pin the current version and a specific date.
   2. Release the cli, inform avd admins, and wait a while.
   3. Make the new layer the default and release the cli.
