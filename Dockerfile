FROM alpine:latest

# Install ca-certificates for HTTPS and sqlite for database operations
RUN apk --no-cache add ca-certificates sqlite tzdata

# Set working directory
WORKDIR /root/

# Copy the binary from the build context
COPY shannon /usr/local/bin/shannon

# Make sure the binary is executable
RUN chmod +x /usr/local/bin/shannon

# Create directories for Shannon data
RUN mkdir -p /root/.config/shannon /root/.local/share/shannon

# Set up environment
ENV PATH="/usr/local/bin:$PATH"

# Expose volume for persistent data
VOLUME ["/root/.config/shannon", "/root/.local/share/shannon"]

# Default command
ENTRYPOINT ["shannon"]
CMD ["--help"]