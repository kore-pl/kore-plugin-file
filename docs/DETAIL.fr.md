# kore-plugin-file — guide d'utilisation

Plugin de gestion de fichiers (L2+) : **19 méthodes réelles** dans une racine contrôlée — lister, lire, écriture atomique, suppression (corbeille par défaut), mkdir, métadonnées, téléversement/téléchargement, extraction/compression, chmod, déplacement, renommage, copie, recherche, liste/restauration/vidage de la corbeille et opérations par lot. Aucun binaire externe unzip/tar/chmod n'est intégré ; les archives passent par la bibliothèque standard Go.

## Installation et compatibilité

Installez le tar.gz publié via l'installateur de plugins kore ; compilez séparément avec `make build` / `make dist`. Protocole `kore.plugin.v1`, `lifecycle=lazy`, `depth_level=production`, `requires_kore: >=0.1.0 <0.2.0`. Capacités du plugin `system:read` + `system:admin` ; les capacités par méthode marquent 6 méthodes de lecture `system:read` et les autres `system:admin`, conformément à `info.json`. Les noms utilisent toujours `file.<verbe>` ; les noms nus ne subsistent que pour compatibilité historique.

## Méthodes

| Méthode | Capacité | Entrée | Résultat clé |
| --- | --- | --- | --- |
| `file.list` | read | `{path}` | name/path/is_dir/size/mode/mod_time/is_symlink/uid/gid ; `.trash` masqué à la racine |
| `file.read` | read | `{path, max_bytes?}` | content UTF-8/size/truncated/is_binary ; binaire sans content |
| `file.stat` | read | `{path}` | size/mode/owner/mod_time/is_dir/is_symlink |
| `file.download` | read | `{path}` | filename/size_bytes/data_base64 ; hors limite, utiliser l'archive du répertoire |
| `file.search` | read | `{path?,pattern,content?,...}` | scanned/matched/skipped/truncated/entries avec line/preview |
| `file.trash.list` | read | `{limit?}` | count/total/skipped/truncated/entries en ordre inverse de suppression |
| `file.write` | admin | `{path,content,create_if_missing?}` | fichier temporaire + renommage atomique |
| `file.delete` | admin | `{path,recursive?,permanent?}` | corbeille par défaut ; `permanent=true` supprime directement |
| `file.mkdir` | admin | `{path,mode?}` | 0755 par défaut |
| `file.chmod` | admin | `{path,mode,recursive?}` | validation de l'arbre entier avant application |
| `file.move` / `file.rename` / `file.copy` | admin | source/cible ou path/name | déplacement entre répertoires, renommage seul, copie récursive ; liens reconstruits |
| `file.upload` / `file.uploadChunk` / `file.uploadStatus` | admin/read | `{path,filename,data_base64,offset?}` | remplacement atomique, envoi reprenable (offset répété idempotent) |
| `file.extract` / `file.compress` | admin | `{path,...}` | bibliothèque standard Go ; zip malveillant rejeté |
| `file.trash.restore` / `file.trash.purge` / `file.batch` | admin | ids / tableaux d'opérations | restauration, vidage irréversible, erreurs par élément en lot |

## Limite de sécurité

La racine contrôlée est définie par `KORE_FILE_ROOT` ; traversée de chemin, NUL, échappement par lien symbolique et zone réservée `.trash` sont rejetés. La corbeille peut être déplacée via `KORE_FILE_TRASH`. Les messages ne contiennent que l'errno et ne divulguent jamais de chemin absolu de l'hôte. Quatre classes d'injection de fautes : kill -9 / timeout / disque plein / limite cgroup.

## Compilation et vérification

```bash
make check   # gofmt + go vet + go test -timeout 200s (y compris race)
make build && make dist
```

## Sécurité et contribution

N'ouvrez pas d'issue publique pour une vulnérabilité ; signalez-la selon [SECURITY.md](SECURITY.md). Voir [CONTRIBUTING.md](CONTRIBUTING.md). Licence MIT.
