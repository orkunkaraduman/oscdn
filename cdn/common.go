package cdn

type Origin struct {
	Scheme        string
	Host          string
	HostOverride  bool
	IgnoreQuery   bool
	HttpsRedirect bool
	UploadBurst   int64
	UploadRate    int64
}
