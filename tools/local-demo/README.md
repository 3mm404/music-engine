# Demo local: fuera del flujo beta

Esta herramienta conserva la consola de zonas A/B para desarrollo. No es el agente que usa Desktop y no consume la configuración de Laravel.

Desde la raíz del repositorio:

```powershell
go run ./tools/local-demo -file C:/ruta/mi-audio.mp3
```

Sin `-file`, busca `tools/local-demo/demo.mp3`. No se distribuye música. Su consola está en `tools/local-demo/console`, junto a su prueba. El agente productivo sigue en `cmd/agent`.

Se movió desde `cmd/engine` para separar claramente herramienta local y entrada beta. Las instrucciones anteriores con `go run ./cmd/engine` deben sustituirse por el comando de esta guía.
