FROM golang:1.18-alpine AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 go build -o mediathek

FROM gcr.io/distroless/static-debian11

WORKDIR /

COPY --from=build /app/mediathek /mediathek

USER nonroot:nonroot

ENTRYPOINT ["/mediathek"]
