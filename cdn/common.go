package cdn

type HostConfig struct {
	Origin struct {
		Scheme string
		Host   string
	}
	HttpsRedirect      bool
	HttpsRedirectPort  int
	DomainOverride     bool
	IgnoreQuery        bool
	CompressionMaxSize int64
	UploadBurst        int64
	UploadRate         int64
}
