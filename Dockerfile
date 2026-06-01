FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/llavero ./cmd/llavero
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/llavero-cli ./cmd/llavero-cli

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /data
COPY --from=build /out/llavero /usr/local/bin/llavero
COPY --from=build /out/llavero-cli /usr/local/bin/llavero-cli
EXPOSE 6380
ENTRYPOINT ["/usr/local/bin/llavero"]
CMD ["-addr", "0.0.0.0:6380", "-snapshot", "/data/llavero.snapshot", "-max-connections", "10000", "-read-timeout", "300s", "-write-timeout", "60s"]
