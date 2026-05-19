# hello-skill

A minimal docker-runtime example skill. It reads `{"name": "..."}` from stdin
and writes `{"message": "hello, ..."}` to stdout.

Publish it (once the CLI lands):

```bash
cd examples/hello-skill
skill push
```

Run it locally for development:

```bash
echo '{"name":"world"}' | python -m hello
# -> {"message": "hello, world"}
```
