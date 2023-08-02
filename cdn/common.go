package cdn

type HostConfig struct {
	Origin struct {
		Scheme string
		Host   string
	}
	DomainOverride bool
	IgnoreQuery    bool
	HttpsRedirect  bool
	UploadBurst    int64
	UploadRate     int64
}
