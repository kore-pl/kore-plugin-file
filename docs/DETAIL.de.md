# kore-plugin-file — Anwendungshandbuch

Dateiverwaltungs-Plugin (L2+): **19 echte Dateimethoden** in einem kontrollierten Wurzelverzeichnis — Auflisten, Lesen, atomares Schreiben, Löschen (standardmäßig Papierkorb), mkdir, Metadaten, Hochladen/Herunterladen, Extrahieren/Komprimieren, chmod, Verschieben, Umbenennen, Kopieren, Suche, Papierkorb-Liste/Restart/Leerung und Stapeloperationen. Keine externen unzip/tar/chmod-Binaries; Archive nutzen die Go-Standardbibliothek.

## Installation und Kompatibilität

Installieren Sie das veröffentlichte tar.gz mit dem kore-Plugin-Installer oder bauen Sie eigenständig (`make build` / `make dist`). Protokoll `kore.plugin.v1`, `lifecycle=lazy`, `depth_level=production`, `requires_kore: >=0.1.0 <0.2.0`. Plugin-Capabilities `system:read` + `system:admin`; Method-Capabilities markieren 6 Lesemethoden als `system:read`, die übrigen als `system:admin`, passend zu `info.json`. Methodennamen verwenden immer `file.<Verb>`; nackte Namen bleiben nur aus Kompatibilitätsgründen.

## Methoden

| Methode | Capability | Eingabe | Wichtigstes Ergebnis |
| --- | --- | --- | --- |
| `file.list` | read | `{path}` | name/path/is_dir/size/mode/mod_time/is_symlink/uid/gid ; `.trash` an der Wurzel verborgen |
| `file.read` | read | `{path, max_bytes?}` | UTF-8 content/size/truncated/is_binary ; Binärdatei ohne content |
| `file.stat` | read | `{path}` | size/mode/owner/mod_time/is_dir/is_symlink |
| `file.download` | read | `{path}` | filename/size_bytes/data_base64 ; bei Größenüberschreitung Verzeichnisarchiv verwenden |
| `file.search` | read | `{path?,pattern,content?,...}` | scanned/matched/skipped/truncated/entries mit line/preview |
| `file.trash.list` | read | `{limit?}` | count/total/skipped/truncated/entries in umgekehrter Löschreihenfolge |
| `file.write` | admin | `{path,content,create_if_missing?}` | temporäre Datei + atomares rename |
| `file.delete` | admin | `{path,recursive?,permanent?}` | standardmäßig Papierkorb ; `permanent=true` löscht direkt |
| `file.mkdir` | admin | `{path,mode?}` | Standard 0755 |
| `file.chmod` | admin | `{path,mode,recursive?}` | ganzer Baum wird vor dem Anwenden geprüft |
| `file.move` / `file.rename` / `file.copy` | admin | Quelle/Ziel oder path/name | verzeichnisübergreifendes Verschieben, nur Umbenennen, rekursives Kopieren ; Symlinks neu erzeugt |
| `file.upload` / `file.uploadChunk` / `file.uploadStatus` | admin/read | `{path,filename,data_base64,offset?}` | atomarer Ersatz, fortsetzbarer Upload (wiederholter offset idempotent) |
| `file.extract` / `file.compress` | admin | `{path,...}` | Go-Standardbibliothek ; bösartiges zip abgelehnt |
| `file.trash.restore` / `file.trash.purge` / `file.batch` | admin | ids / Operationsarrays | Wiederherstellen, unwiderrufliches Leeren, Fehler pro Element im Batch |

## Sicherheitsgrenze

Das kontrollierte Wurzelverzeichnis wird durch `KORE_FILE_ROOT` gesetzt; Pfad-Traversal, NUL, Symlink-Escapes und der reservierte Bereich `.trash` werden abgelehnt. Der Papierkorb lässt sich über `KORE_FILE_TRASH` verschieben. Fehlermeldungen enthalten nur errno und niemals absolute Host-Pfade. Vier Fehlerinjektionsklassen: kill -9 / Timeout / volle Platte / cgroup-Limit.

## Bauen und prüfen

```bash
make check   # gofmt + go vet + go test -timeout 200s (inkl. Race-Gate)
make build && make dist
```

## Sicherheit und Mitwirkung

Melden Sie Schwachstellen nicht in öffentlichen Issues, sondern gemäß [SECURITY.md](SECURITY.md). Beiträge: [CONTRIBUTING.md](CONTRIBUTING.md). MIT-Lizenz.
