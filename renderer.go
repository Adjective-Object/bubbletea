package tea

// Renderer is the interface for Bubble Tea renderers.
type Renderer interface {
	// Start the renderer.
	start()

	// Stop the renderer, but render the final frame in the buffer, if any.
	stop()

	// Stop the renderer without doing any final rendering.
	kill()

	// Write a frame to the renderer. The renderer can write this data to
	// output at its discretion.
	write(string)

	// Request a full re-render.
	repaint()

	// Whether or not the alternate screen buffer is enabled.
	altScreen() bool

	// Record internally that the alternate screen buffer is enabled. This
	// does not actually toggle the alternate screen buffer.
	setAltScreen(bool)
}
