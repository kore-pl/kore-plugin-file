# kore-plugin-file — guia de uso

Plugin de gerenciamento de arquivos (L2+): **19 métodos reais** dentro de uma raiz controlada — listar, ler, gravação atômica, excluir (por padrão para a lixeira), mkdir, metadados, upload/download, extrair/compactar, chmod, mover, renomear, copiar, busca, lista/restauração/limpeza da lixeira e operações em lote. Não inclui binários externos unzip/tar/chmod; arquivos compactados usam a biblioteca padrão do Go.

## Instalação e compatibilidade

Instale o tar.gz publicado com o instalador de plugins do kore ou compile separadamente (`make build` / `make dist`). Protocolo `kore.plugin.v1`, `lifecycle=lazy`, `depth_level=production`, `requires_kore: >=0.1.0 <0.2.0`. Capacidades do plugin `system:read` + `system:admin`; 6 métodos de leitura são `system:read`, os demais `system:admin`, conforme `info.json`. Nomes de métodos sempre `file.<verbo>`; nomes curtos permanecem apenas por compatibilidade histórica.

## Métodos

| Método | Capacidade | Entrada | Resultado principal |
| --- | --- | --- | --- |
| `file.list` | read | `{path}` | name/path/is_dir/size/mode/mod_time/is_symlink/uid/gid ; `.trash` oculto na raiz |
| `file.read` | read | `{path, max_bytes?}` | content UTF-8/size/truncated/is_binary ; binário sem content |
| `file.stat` | read | `{path}` | size/mode/owner/mod_time/is_dir/is_symlink |
| `file.download` | read | `{path}` | filename/size_bytes/data_base64 ; acima do limite use arquivo do diretório |
| `file.search` | read | `{path?,pattern,content?,...}` | scanned/matched/skipped/truncated/entries com line/preview |
| `file.trash.list` | read | `{limit?}` | count/total/skipped/truncated/entries em ordem inversa de exclusão |
| `file.write` | admin | `{path,content,create_if_missing?}` | arquivo temporário + rename atômico |
| `file.delete` | admin | `{path,recursive?,permanent?}` | lixeira por padrão ; `permanent=true` exclui diretamente |
| `file.mkdir` | admin | `{path,mode?}` | padrão 0755 |
| `file.chmod` | admin | `{path,mode,recursive?}` | árvore inteira validada antes de aplicar |
| `file.move` / `file.rename` / `file.copy` | admin | origem/destino ou path/name | mover entre diretórios, apenas renomear, copiar recursivamente ; links simbólicos recriados |
| `file.upload` / `file.uploadChunk` / `file.uploadStatus` | admin/read | `{path,filename,data_base64,offset?}` | substituição atômica, upload retomável (offset repetido idempotente) |
| `file.extract` / `file.compress` | admin | `{path,...}` | biblioteca padrão Go ; zip malicioso rejeitado |
| `file.trash.restore` / `file.trash.purge` / `file.batch` | admin | ids / arrays de operações | restaurar, limpeza irreversível, erros por item em lote |

## Limite de segurança

A raiz controlada é definida por `KORE_FILE_ROOT`; travessia de caminho, NUL, escape por link simbólico e a área reservada `.trash` são rejeitados. A lixeira pode ser alterada via `KORE_FILE_TRASH`. Mensagens de erro contêm apenas errno e nunca expõem caminhos absolutos do host. Quatro classes de injeção de falhas: kill -9 / timeout / disco cheio / limite de cgroup.

## Compilação e verificação

```bash
make check   # gofmt + go vet + go test -timeout 200s (inclui race)
make build && make dist
```

## Segurança e contribuição

Não abra issues públicas para vulnerabilidades; divulgue-as conforme [SECURITY.md](SECURITY.md). Veja [CONTRIBUTING.md](CONTRIBUTING.md). Licença MIT.
