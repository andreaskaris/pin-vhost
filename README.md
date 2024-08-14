## Build instructions

### Containerized

Build image locally:

```
make container-image
```

Run image in foreground:

```
make run-container-foreground
```

Run image in foreground in discovery mode (no pinning):

```
make run-container-foreground-discovery-mode
```

Run image in background:

```
make run-container
```

Remove container running in background:

```
make stop-container
```
