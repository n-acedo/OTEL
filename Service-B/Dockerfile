FROM golang:1.23.2 AS build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o goappB

FROM scratch
WORKDIR /app
COPY --from=build /app/goappB .
ENTRYPOINT ["./goappB"]