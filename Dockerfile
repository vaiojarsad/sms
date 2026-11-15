FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o /out/sms .

FROM alpine:3.22

ENV PORT=8080

RUN apk add --no-cache ca-certificates \
    && addgroup -S sms \
    && adduser -S -G sms sms

COPY --from=build /out/sms /usr/local/bin/sms

USER sms
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/sms"]
