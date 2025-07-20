FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod .
COPY main.go .
COPY index.html .
RUN go build -o app main.go

FROM alpine:3.19
WORKDIR /app
COPY --from=build /src/app ./app
COPY --from=build /src/index.html ./index.html
EXPOSE 8080
ENV TURNSTILE_SECRET=""
CMD ["/app/app"]
