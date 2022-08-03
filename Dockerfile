FROM golang:1.18

WORKDIR /shopping-list

COPY . ./

RUN go build -o /app .

CMD ["/app"]