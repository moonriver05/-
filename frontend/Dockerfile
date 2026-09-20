FROM node:22-alpine AS build
WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN corepack enable && corepack prepare pnpm@10.17.1 --activate && pnpm install --frozen-lockfile
COPY . .
ARG VITE_WIKI_IMAGE_MODE=external
ARG VITE_MEDIA_MANIFEST_URL=
ARG VITE_MEDIA_ALLOWED_ORIGINS=
ENV VITE_WIKI_IMAGE_MODE=$VITE_WIKI_IMAGE_MODE
ENV VITE_MEDIA_MANIFEST_URL=$VITE_MEDIA_MANIFEST_URL
ENV VITE_MEDIA_ALLOWED_ORIGINS=$VITE_MEDIA_ALLOWED_ORIGINS
RUN pnpm run build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
