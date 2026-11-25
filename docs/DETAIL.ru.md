# kore-plugin-file — руководство по использованию

Плагин управления файлами (L2+): **19 реальных файловых методов** в контролируемом корневом каталоге — список, чтение, атомарная запись, удаление (по умолчанию в корзину), mkdir, метаданные, загрузка/выгрузка, распаковка/архивация, chmod, перемещение, переименование, копирование, поиск, список/восстановление/очистка корзины и пакетные операции. Внешние утилиты unzip/tar/chmod не поставляются; архивы обрабатываются стандартной библиотекой Go.

## Установка и совместимость

Устанавливайте опубликованный tar.gz через установщик плагинов kore; собирайте отдельно (`make build` / `make dist`). Протокол `kore.plugin.v1`, `lifecycle=lazy`, `depth_level=production`, `requires_kore: >=0.1.0 <0.2.0`. Возможности плагина `system:read` + `system:admin`; 6 методов чтения помечены `system:read`, остальные — `system:admin`, в соответствии с `info.json`. Имена методов всегда `file.<глагол>`; короткие имена оставлены для совместимости.

## Методы

| Метод | Возможность | Вход | Ключевой результат |
| --- | --- | --- | --- |
| `file.list` | read | `{path}` | name/path/is_dir/size/mode/mod_time/is_symlink/uid/gid ; `.trash` скрыт в корне |
| `file.read` | read | `{path, max_bytes?}` | UTF-8 content/size/truncated/is_binary ; бинарник без content |
| `file.stat` | read | `{path}` | size/mode/owner/mod_time/is_dir/is_symlink |
| `file.download` | read | `{path}` | filename/size_bytes/data_base64 ; при превышении лимита архив каталога |
| `file.search` | read | `{path?,pattern,content?,...}` | scanned/matched/skipped/truncated/entries с line/preview |
| `file.trash.list` | read | `{limit?}` | count/total/skipped/truncated/entries в обратном порядке удаления |
| `file.write` | admin | `{path,content,create_if_missing?}` | временный файл + атомарный rename |
| `file.delete` | admin | `{path,recursive?,permanent?}` | по умолчанию корзина ; `permanent=true` удаляет сразу |
| `file.mkdir` | admin | `{path,mode?}` | по умолчанию 0755 |
| `file.chmod` | admin | `{path,mode,recursive?}` | весь каталог проверяется до применения |
| `file.move` / `file.rename` / `file.copy` | admin | источник/назначение или path/name | перемещение между каталогами, только переименование, рекурсивное копирование ; символические ссылки пересоздаются |
| `file.upload` / `file.uploadChunk` / `file.uploadStatus` | admin/read | `{path,filename,data_base64,offset?}` | атомарная замена, возобновляемая загрузка (повтор offset идемпотентен) |
| `file.extract` / `file.compress` | admin | `{path,...}` | стандартная библиотека Go ; вредоносный zip отклоняется |
| `file.trash.restore` / `file.trash.purge` / `file.batch` | admin | ids / массивы операций | восстановление, безвозвратная очистка, ошибки по элементам в пакете |

## Граница безопасности

Контролируемый корень задаётся `KORE_FILE_ROOT`; обход пути, NUL, выход по символической ссылке и зарезервированная зона `.trash` отклоняются. Корзину можно перенести через `KORE_FILE_TRASH`. Сообщения об ошибках содержат только errno и никогда не раскрывают абсолютные пути хоста. Четыре класса инъекции отказов: kill -9 / таймаут / заполненный диск / лимит cgroup.

## Сборка и проверка

```bash
make check   # gofmt + go vet + go test -timeout 200s (включая race)
make build && make dist
```

## Безопасность и участие

Не открывайте публичные issue по уязвимостям; сообщайте приватно согласно [SECURITY.md](SECURITY.md). Участие: [CONTRIBUTING.md](CONTRIBUTING.md). Лицензия MIT.
