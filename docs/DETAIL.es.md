# kore-plugin-file: guía de uso

Plugin de gestión de archivos (L2+): **19 métodos reales** dentro de una raíz controlada — listar, leer, escritura atómica, borrar (por defecto a papelera), crear directorio, metadatos, subir/descargar, extraer/comprimir, cambiar permisos, mover, renombrar, copiar, buscar, lista/restaurar/vaciar papelera y operaciones por lote. No incluye binarios externos unzip/tar/chmod; los archivos comprimidos usan la biblioteca estándar de Go.

## Instalación y compatibilidad

Instale el tar.gz publicado con el instalador de plugins de kore; compílelo por separado con `make build` / `make dist`. Protocolo `kore.plugin.v1`, `lifecycle=lazy`, `depth_level=production`, `requires_kore: >=0.1.0 <0.2.0`. Capacidades del plugin `system:read` + `system:admin`; las capacidades por método marcan 6 métodos de lectura `system:read` y el resto `system:admin`, tal como en `info.json`. Los nombres usan siempre `file.<verbo>`; los nombres simples solo son compatibilidad histórica.

## Métodos

| Método | Capacidad | Entrada | Resultado clave |
| --- | --- | --- | --- |
| `file.list` | read | `{path}` | name/path/is_dir/size/mode/mod_time/is_symlink/uid/gid; `.trash` oculto en la raíz |
| `file.read` | read | `{path, max_bytes?}` | content UTF-8/size/truncated/is_binary; binario sin content |
| `file.stat` | read | `{path}` | size/mode/owner/mod_time/is_dir/is_symlink |
| `file.download` | read | `{path}` | filename/size_bytes/data_base64; si excede el límite, use archivo de directorio |
| `file.search` | read | `{path?,pattern,content?,...}` | scanned/matched/skipped/truncated/entries con line/preview |
| `file.trash.list` | read | `{limit?}` | count/total/skipped/truncated/entries en orden inverso de borrado |
| `file.write` | admin | `{path,content,create_if_missing?}` | archivo temporal + rename atómico |
| `file.delete` | admin | `{path,recursive?,permanent?}` | papelera por defecto; `permanent=true` borra directamente |
| `file.mkdir` | admin | `{path,mode?}` | 0755 por defecto |
| `file.chmod` | admin | `{path,mode,recursive?}` | valida todo el árbol antes de aplicar |
| `file.move` / `file.rename` / `file.copy` | admin | origen/destino o path/name | movimiento entre directorios, solo renombrar, copia recursiva; enlaces reconstruidos |
| `file.upload` / `file.uploadChunk` / `file.uploadStatus` | admin/read | `{path,filename,data_base64,offset?}` | reemplazo atómico, subida reanudable (offset repetido idempotente) |
| `file.extract` / `file.compress` | admin | `{path,...}` | biblioteca estándar de Go; zip malicioso rechazado |
| `file.trash.restore` / `file.trash.purge` / `file.batch` | admin | ids / arrays de operación | restaurar, vaciado irreversible, errores por elemento en lote |

## Límite de seguridad

La raíz controlada la fija `KORE_FILE_ROOT`; se rechazan traversing de rutas, NUL, escapes por enlaces simbólicos y el área reservada `.trash`. La papelera puede cambiarse con `KORE_FILE_TRASH`. Los mensajes solo incluyen errno y nunca filtran rutas absolutas del host. Cuatro clases de inyección de fallos: kill -9 / timeout / disco lleno / límite cgroup.

## Compilación y verificación

```bash
make check   # gofmt + go vet + go test -timeout 200s (incluye race)
make build && make dist
```

## Seguridad y contribución

No abra issues públicos para vulnerabilidades; infórmelas según [SECURITY.md](SECURITY.md). Vea [CONTRIBUTING.md](CONTRIBUTING.md). Licencia MIT.
