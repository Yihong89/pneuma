FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pneuma ./cmd/pneuma \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/pneuma /pneuma
# Owned by the nonroot user so a fresh named volume inherits it.
COPY --from=build --chown=65532:65532 /out/data /data
ENV PNEUMA_DATA_DIR=/data
VOLUME /data
ENTRYPOINT ["/pneuma"]
