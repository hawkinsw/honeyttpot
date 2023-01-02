### Creating a self-signed Certificate and Key

```console
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -sha256 -days 365 -nodes
```

### Building

```console
go build -o http honeyttpot.go
```

