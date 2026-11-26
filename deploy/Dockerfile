FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/backpack ./cmd/backpack
# The files directory: a new named volume is seeded from the image, owner
# included, so it must exist here, owned by the nonroot user (uid 65532) the
# final image runs as. Otherwise the volume starts root-owned and unwritable.
RUN mkdir -p /out/files

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/backpack /backpack
COPY --from=build --chown=65532:65532 /out/files /data/files
EXPOSE 8780
ENTRYPOINT ["/backpack"]
CMD ["serve"]
