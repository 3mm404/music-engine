package audio

// SharedSystemOutput identifies a backend that mixes zones on the system stereo
// output, rather than assigning independent physical channels.
type SharedSystemOutput interface{ SharedStereo() bool }

func IsSharedStereo(output Output) bool {
	c, ok := output.(SharedSystemOutput)
	return ok && c.SharedStereo()
}
