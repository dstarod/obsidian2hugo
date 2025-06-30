
"""
Конвертер из Obsidian в Hugo
===========================

Этот скрипт конвертирует коллекцию заметок из Obsidian в формат, подходящий для
генератора статичных сайтов Hugo, в частности, используя структуру Page Bundle.

Возможности:
------------
- **Управление Front Matter**: Автоматически создает или обновляет YAML front matter.
  - Гарантирует наличие поля `title`, используя имя файла, если оно отсутствует.
  - Гарантирует наличие поля `date`, используя текущую временную метку, если оно отсутствует.
- **Создание Page Bundle**: Для каждой заметки создается каталог с ее именем.
  Сама заметка переименовывается в `index.md` внутри этого каталога.
- **Обработка вложений**:
  - Находит все встроенные вложения (например, `![[image.png]]`).
  - Копирует их из центрального каталога вложений в Page Bundle заметки.
  - Переименовывает вложения в формат `md5_хэш.расширение` для предотвращения конфликтов имен.
  - Обновляет ссылки в файле `index.md`, чтобы они указывали на новые имена вложений
    (например, `![](md5_хэш.png)`).

Использование:
--------------
Скрипт предназначен для запуска из командной строки.

Синтаксис:
  python obsidian_to_hugo.py --notes-dir <путь> --attachments-dir <путь> --hugo-posts-dir <путь>

Аргументы:
  --notes-dir         (Обязательный) Абсолютный путь к каталогу с вашими заметками
                      Obsidian (.md файлы).
  --attachments-dir   (Обязательный) Абсолютный путь к каталогу, где Obsidian
                      хранит все вложения (изображения, PDF и т.д.).
  --hugo-posts-dir    (Обязательный) Абсолютный путь к целевому каталогу для контента
                      Hugo (например, `/путь/к/сайту-hugo/content/posts`).
  --filter-tag        (Опциональный) Тег, по которому отбираются заметки.
                      По умолчанию: 'blog'.
  --remove-filter-tag (Опциональный) Флаг, указывающий на необходимость удалить
                      тег фильтрации из финального списка тегов.
  --exclude-dirs      (Опциональный) Список каталогов для исключения из поиска.
                      Можно указать несколько через пробел.
  --log-level         (Опциональный) Уровень логирования (DEBUG, INFO, WARNING, ERROR).
                      По умолчанию: INFO.

Пример:
  python obsidian_to_hugo.py \
      --notes-dir "/Users/user/Documents/ObsidianVault/Notes" \
      --attachments-dir "/Users/user/Documents/ObsidianVault/Attachments" \
      --hugo-posts-dir "/Users/user/Code/hugo-blog/content/posts" \
      --exclude-dirs Templates Private \
      --log-level DEBUG
"""

# Стандартные библиотеки
import argparse
import hashlib
import logging
import re
import shutil
from datetime import datetime, timedelta, timezone
from pathlib import Path

# Сторонние библиотеки
import yaml

# Паттерн для поиска блока YAML Front Matter в начале файла.
# Он ищет блок, начинающийся и заканчивающийся на '---'.
# re.DOTALL позволяет '.' соответствовать символу новой строки.
# Используем нежадный поиск (.*?), чтобы захватить только первый блок.
FRONT_MATTER_PATTERN = re.compile(r'^---\s*$(.*?)^---\s*', re.DOTALL | re.MULTILINE)

def calculate_md5(file_path):
    """Вычисляет MD5-хэш файла."""
    hash_md5 = hashlib.md5()
    try:
        with open(file_path, "rb") as f:
            for chunk in iter(lambda: f.read(4096), b""):
                hash_md5.update(chunk)
        return hash_md5.hexdigest()
    except FileNotFoundError:
        logging.warning(f"Файл для хэширования не найден: {file_path}")
        return None

def parse_note_content(full_content):
    """Извлекает YAML front matter и основное содержимое с помощью regex."""
    match = FRONT_MATTER_PATTERN.match(full_content)
    if not match:
        # Front matter не найден, возвращаем пустые свойства и полный контент
        return {}, full_content

    yaml_content = match.group(1)
    # Контент - это все, что идет после найденного блока
    note_body = full_content[match.end():].lstrip()

    try:
        properties = yaml.safe_load(yaml_content) or {}
        return properties, note_body
    except yaml.YAMLError as e:
        logging.warning(f"Ошибка парсинга YAML: {e}. Блок будет проигнорирован.")
        # В случае ошибки парсинга, считаем, что front matter нет
        return {}, full_content

def process_notes(notes_dir, attachments_dir, hugo_posts_dir, filter_tag, remove_filter_tag, exclude_dirs):
    """
    Основная функция для обработки заметок.
    """
    hugo_posts_dir.mkdir(exist_ok=True)
    logging.info(f"Рекурсивно сканирую заметки в: {notes_dir}")

    abs_exclude_paths = [notes_dir.joinpath(d).resolve() for d in (exclude_dirs or [])]
    if abs_exclude_paths:
        logging.info(f"Исключаю каталоги: {[str(p) for p in abs_exclude_paths]}")

    for note_path in notes_dir.rglob('*.md'):
        if any(note_path.resolve().is_relative_to(p) for p in abs_exclude_paths):
            logging.debug(f"Пропускаю заметку из исключенного каталога: {note_path}")
            continue

        logging.info(f"--- Проверяю заметку: {note_path.relative_to(notes_dir)} ---")

        with open(note_path, 'r', encoding='utf-8') as f:
            full_content = f.read()

        properties, content = parse_note_content(full_content)

        # --- ПРОВЕРКА ТЕГА ---
        tags_from_note = properties.get('tags', [])
        if isinstance(tags_from_note, str):
            tags_list = [tag.strip() for tag in tags_from_note.split(',')]
        else:
            tags_list = tags_from_note or []

        if filter_tag not in tags_list:
            logging.debug(f"Пропускаю заметку '{note_path.name}', так как у нее нет тега '{filter_tag}'.")
            continue
        # --- КОНЕЦ ПРОВЕРКИ ---

        logging.info(f"Обрабатываю заметку: {note_path.name} (найден тег '{filter_tag}')")

        # --- ОБНОВЛЕНИЕ ТЕГОВ ---
        if remove_filter_tag:
            logging.debug(f"Удаляю тег '{filter_tag}' из списка тегов.")
            updated_tags = [t for t in tags_list if t != filter_tag]
            if updated_tags:
                properties['tags'] = updated_tags
            elif 'tags' in properties:
                del properties['tags']
        else:
            properties['tags'] = tags_list
        # --- КОНЕЦ ОБНОВЛЕНИЯ ---

        # --- ЛОГИКА УПРАВЛЕНИЯ FRONT MATTER ---
        if not properties.get('title'):
            properties['title'] = note_path.stem
            logging.debug(f"Свойство 'title' не найдено. Установлено: '{note_path.stem}'")

        if not properties.get('date'):
            tz_offset = timezone(timedelta(hours=3))
            now_str = datetime.now(tz=tz_offset).isoformat()
            properties['date'] = now_str
            logging.debug(f"Свойство 'date' не найдено. Установлено: '{now_str}'")
        # --- КОНЕЦ ЛОГИКИ ---

        bundle_dir_name = note_path.stem
        target_bundle_dir = hugo_posts_dir / bundle_dir_name
        target_bundle_dir.mkdir(exist_ok=True)
        logging.info(f"Создан/обновлен каталог поста: {target_bundle_dir}")

        attachment_pattern = r'!\[\[(.*?)\]\]'
        replacements = []

        for match in re.finditer(attachment_pattern, content):
            original_link_text = match.group(0)
            original_filename = match.group(1)

            source_attachment_path = attachments_dir / original_filename

            if not source_attachment_path.exists():
                logging.warning(f"Вложение '{original_filename}' не найдено в {attachments_dir}")
                continue

            md5_hash = calculate_md5(source_attachment_path)
            if not md5_hash:
                continue

            extension = source_attachment_path.suffix
            new_filename = f"{md5_hash}{extension}"
            target_attachment_path = target_bundle_dir / new_filename

            logging.debug(f"Копирую вложение: '{original_filename}' -> '{new_filename}'")
            shutil.copy2(source_attachment_path, target_attachment_path)

            new_link_text = f"![]({new_filename})"
            replacements.append((original_link_text, new_link_text))

        if replacements:
            logging.info("Обновляю ссылки в тексте...")
            for old, new in replacements:
                content = content.replace(old, new)

        # --- ОБРАБОТКА ВИКИ-ССЫЛОК ---
        # Заменяем [[вики-ссылка]] на "вики-ссылка".
        # Используем negative lookbehind `(?<!\!)`, чтобы не затрагивать вложения ![[...]].
        wikilink_pattern = r'(?<!\\!)\[\[(.*?)\]\]'
        if re.search(wikilink_pattern, content):
            logging.info("Обновляю вики-ссылки в тексте (удаляю квадратные скобки)...")
            content = re.sub(wikilink_pattern, r'\1', content)
        # --- КОНЕЦ ОБРАБОТКИ ВИКИ-ССЫЛОК ---

        target_note_path = target_bundle_dir / 'index.md'
        with open(target_note_path, 'w', encoding='utf-8') as f:
            yaml_header = yaml.dump(properties, allow_unicode=True, default_flow_style=False, sort_keys=False)
            f.write('---\n')
            f.write(yaml_header)
            f.write('---\n\n')
            f.write(content)

        logging.info(f"Заметка сохранена как: {target_note_path}")

    logging.info("--- Обработка завершена. ---")

def main():
    parser = argparse.ArgumentParser(
        description="Конвертирует заметки Obsidian в формат Hugo Page Bundle.",
        formatter_class=argparse.RawTextHelpFormatter
    )
    parser.add_argument(
        '--notes-dir',
        type=Path,
        required=True,
        help="Путь к каталогу с вашими заметками Obsidian (.md файлы)."
    )
    parser.add_argument(
        '--attachments-dir',
        type=Path,
        required=True,
        help="Путь к каталогу, где хранятся все вложения (изображения и т.д.)."
    )
    parser.add_argument(
        '--hugo-posts-dir',
        type=Path,
        required=True,
        help="Путь к каталогу, куда будут сохраняться посты для Hugo (например, /path/to/hugo/content/posts)."
    )
    parser.add_argument(
        '--filter-tag',
        type=str,
        default='blog',
        help="Тег, по которому будут отбираться заметки для обработки. По умолчанию: 'blog'."
    )
    parser.add_argument(
        '--remove-filter-tag',
        action='store_true',
        help="Если указано, тег, по которому производилась фильтрация, будет удален из итогового списка тегов."
    )
    parser.add_argument(
        '--exclude-dirs',
        type=str,
        nargs='+',
        help="Список имен каталогов, которые нужно исключить из сканирования."
    )
    parser.add_argument(
        '--log-level',
        type=str,
        default='INFO',
        choices=['DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL'],
        help="Уровень логирования (DEBUG, INFO, WARNING, ERROR). По умолчанию: INFO."
    )

    args = parser.parse_args()

    # Настройка логирования
    log_level = getattr(logging, args.log_level.upper(), logging.INFO)
    logging.basicConfig(level=log_level, format='[%(levelname)s] %(message)s')

    if not args.notes_dir.is_dir():
        logging.error(f"Каталог с заметками не найден: {args.notes_dir}")
        return
    if not args.attachments_dir.is_dir():
        logging.error(f"Каталог с вложениями не найден: {args.attachments_dir}")
        return

    process_notes(
        args.notes_dir,
        args.attachments_dir,
        args.hugo_posts_dir,
        args.filter_tag,
        args.remove_filter_tag,
        args.exclude_dirs
    )


if __name__ == '__main__':
    main()
