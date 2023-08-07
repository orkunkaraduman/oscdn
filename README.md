# oscdn - Open Source Content Delivery Network

[![Go Reference](https://pkg.go.dev/badge/github.com/orkunkaraduman/oscdn.svg)](https://pkg.go.dev/github.com/orkunkaraduman/oscdn)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=orkunkaraduman_oscdn&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=orkunkaraduman_oscdn)

oscdn is an open-source Content Delivery Network (CDN) software designed to optimize website performance by efficiently
delivering static assets to users worldwide. It supports HTTP/2, partial content serving, and rate limiting, making it
a powerful and versatile solution for content delivery needs.

## Features

- HTTP/2 Support: oscdn leverages the latest HTTP/2 protocol to improve the loading speed and performance of websites,
reducing latency and increasing concurrent connections.

- HTTP/2 with h2c Support: In addition to traditional HTTP/2 over TLS (HTTP/2), oscdn now supports HTTP/2 Cleartext
(h2c). This feature allows clients to initiate HTTP/2 communication without prior TLS negotiation.
h2c support can be beneficial in certain scenarios where TLS encryption is not required, reducing handshake overhead
and simplifying deployments.

- Partial Content Serving: oscdn allows clients to request only parts of a resource, making it ideal for large files
like videos, audio, or images. This feature enhances the overall user experience and saves bandwidth.

- Rate Limiting: Control and manage traffic flow with rate limiting. oscdn provides configurable rate limits to prevent
abuse and ensure fair usage of resources.

## Getting Started

Follow the steps below to get oscdn up and running on your server:

1. Prerequisites: Ensure you have go with version 1.20 installed on your system.

2. Installation: Install oscdn by running the following command: `go get github.com/orkunkaraduman/oscdn@latest`

3. Configuration: You can view command-line flags and `config.yaml` at this time. 

4. Start the Server: Launch the oscdn server by running the following command: `oscdn --store-path=/example/store/path`

## Contributing

We welcome contributions from the community to improve and expand oscdn's capabilities. If you find a bug, have a
feature request, or want to contribute code, please follow our guidelines for contributing
([CONTRIBUTING.md](CONTRIBUTING.md)) and submit a pull request.

## License

oscdn is open-source software released under the [BSD 3-Clause License](https://opensource.org/licenses/BSD-3-Clause).

## Acknowledgments

We would like to thank the open-source community and the developers of the libraries and tools that oscdn depends on.
Your contributions help make oscdn a reliable and powerful CDN solution.

## Contact

If you have any questions, suggestions, or need support, you can reach me at [ok@orkunkaraduman.com](mailto:ok@orkunkaraduman.com).
