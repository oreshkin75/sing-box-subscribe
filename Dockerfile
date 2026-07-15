FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /sing-box-subscribe .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /sing-box-subscribe /sing-box-subscribe
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/sing-box-subscribe"]
