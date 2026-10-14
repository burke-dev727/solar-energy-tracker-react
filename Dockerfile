# Builds one image serving the React frontend and the Go API from a data snapshot.
# Create snapshot.json first: cd server && go run . export-snapshot ../snapshot.json

FROM node:24-alpine AS web
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html vite.config.ts tsconfig.json tsconfig.app.json tsconfig.node.json ./
COPY public ./public
COPY src ./src
RUN npm run build

FROM golang:1.27-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/*.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /energy-api .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=server /energy-api ./energy-api
COPY --from=web /app/dist ./dist
COPY --chmod=0644 snapshot.json ./snapshot.json
ENV DATA_SOURCE=snapshot \
    SNAPSHOT_PATH=/app/snapshot.json \
    STATIC_DIR=/app/dist \
    TRUST_PROXY_HEADERS=true \
    PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/energy-api"]
