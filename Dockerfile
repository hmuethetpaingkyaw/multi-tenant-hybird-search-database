FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY api ./api
COPY apperror ./apperror
COPY config ./config
COPY constants ./constants
COPY domain ./domain
COPY driver ./driver
COPY http ./http
COPY repository ./repository
COPY usecase ./usecase
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.21
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8080
ENTRYPOINT ["api"]
