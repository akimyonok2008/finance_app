# Multi-stage: `dev` (default, used by docker-compose for local development —
# hot reload via the Vite dev server) and `production` (a real static build
# served by an unprivileged nginx, not a dev server). Build with
# `--target production` for anything other than local development.

FROM node:24-alpine AS dev

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]

# ---- build: compiles the static production assets ---------------------------
FROM node:24-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# VITE_-prefixed vars are inlined into the built JS at build time, not read at
# container start, so they must be supplied as a build ARG here.
ARG VITE_API_BASE_URL
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
RUN npm run build

# ---- production: static assets served by nginx, non-root by default --------
FROM nginxinc/nginx-unprivileged:1.27-alpine AS production

COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 8080
