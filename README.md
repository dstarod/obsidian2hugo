# obsidian2hugo

Скрипт конвертации заметок [Obsidian](https://obsidian.md) в посты для движка [Hugo](https://gohugo.io) в формате [Page Bundles](https://gohugo.io/content-management/page-bundles/).

Встроенные изображения в формате `![[Image.png]]` преобразуются в Markdown-ссылки формата `![](md5_hash_Image_name.png)`.

Вики-ссылки вида `[[Заметка]]` преобразуются в простой текст `Заметка`.

## Параметры запуска

- `--notes-dir`: Путь к каталогу с вашими заметками Obsidian (.md файлы)
- `--attachments-dir`: Путь к каталогу, где хранятся все вложения (изображения и т.д.)
- `--hugo-posts-dir`: Путь к каталогу, куда будут сохраняться посты для Hugo (например, /path/to/hugo/content/posts)
- `--filter-tag`: Тег, по которому будут отбираться заметки для обработки. По умолчанию: 'blog'
- `--remove-filter-tag`: Если указано, тег, по которому производилась фильтрация, будет удален из итогового списка тегов
- `--exclude-dirs`: Список имен каталогов, которые нужно исключить из сканирования
- `--log-level`: Уровень логирования (DEBUG, INFO, WARNING, ERROR). По умолчанию: INFO

В качестве `--notes-dir` можно указать путь к хранилищу Obsidian, а `--exclude-dirs` может указывать на каталог с шаблонами и картинками. Например:

```bash
python obsidian2hugo.py \
    --notes-dir "/path/vault" \
    --attachments-dir "/path/vault/Cache" \
    --hugo-posts-dir "/path/hugo/content/posts" \
    --log-level WARNING \
    --filter-tag blog \
    --remove-filter-tag \
    --exclude-dirs Templates Cache
```

Можно выбрать путь не к корневой папке с хранилищем, а к разделу со статьями, которые вы собираетесь публиковать, чтобы не сканировать все заметки.

## Сборка

По функционалу версии на Python и Go идентичны, используйте какая больше нравится.

### Версия на Python

Устанавливаем зависимость `pyyaml`, на этом всё.

```bash
pip install pyyaml
python obsidian2hugo.py ....
```

Или, если предпочитаете более цивилизованный способ, используя `uv`

```bash
uv init
uv python pin 3.13
uv add pyyaml
uv run obsidian2hugo.py ....
```

### Версия на Go

Сборка и запуск:

```
go mod init obsidian2hugo
go mod tidy
go build
./obsidian2hugo ....
```