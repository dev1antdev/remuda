// The Russian words of the toast after a worktree is made: what it got from the repository's .worktreeinclude.

import type { Dictionary } from "../../i18n";

export const worktreeinclude: Dictionary = {
  "The worktree is made, but the files .worktreeinclude names were not copied: {error}": "Worktree создан, но файлы из .worktreeinclude не скопированы: {error}",
  ".worktreeinclude matches {n} ignored files, more than a worktree gets: nothing was copied — narrow its patterns": ".worktreeinclude подходит под {n} игнорируемых файлов — больше, чем копируется в worktree: ничего не скопировано, сузьте шаблоны",
  "Nothing copied from .worktreeinclude: {n} files are already in the worktree or lead out of it": [
    "Из .worktreeinclude ничего не скопировано: {n} файл уже есть в worktree или ведёт за его пределы",
    "Из .worktreeinclude ничего не скопировано: {n} файла уже есть в worktree или ведут за его пределы",
    "Из .worktreeinclude ничего не скопировано: {n} файлов уже есть в worktree или ведут за его пределы",
  ],
  ".worktreeinclude matches no ignored file: nothing to copy": ".worktreeinclude не подходит ни под один игнорируемый файл: копировать нечего",
  "Copied {n} files from .worktreeinclude: {files}": ["Скопирован {n} файл из .worktreeinclude: {files}", "Скопировано {n} файла из .worktreeinclude: {files}", "Скопировано {n} файлов из .worktreeinclude: {files}"],
};
