package cdn

type HostConfig struct {
	Origin struct {
		Scheme string
		Host   string
	}
	HttpsRedirect     bool
	HttpsRedirectPort int
	DomainOverride    bool
	IgnoreQuery       bool
	CompressMimeTypes []string
	UploadBurst       int64
	UploadRate        int64
}
