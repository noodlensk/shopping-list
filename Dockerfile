FROM golang:1.24

WORKDIR /shopping-list

COPY . ./

RUN go build -o /app .

CMD ["/app"]