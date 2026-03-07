#!/usr/bin/env bash

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}         Axis Git Push Helper           ${NC}"
echo -e "${CYAN}========================================${NC}"

if [ ! -f "README.md" ]; then
  echo -e "${RED}Error: run this script in the Axis project root.${NC}"
  exit 1
fi

if [ ! -d ".git" ]; then
  echo -e "${RED}Error: current directory is not a Git repository.${NC}"
  echo -e "${YELLOW}Hint:${NC} run:"
  echo "  git init"
  echo "  git checkout -b main"
  exit 1
fi

BRANCH="$(git branch --show-current 2>/dev/null || true)"
if [ -z "${BRANCH}" ]; then
  BRANCH="main"
fi

echo -e "${BLUE}Current branch:${NC} ${BRANCH}"

HAS_ORIGIN="false"
if git remote get-url origin >/dev/null 2>&1; then
  HAS_ORIGIN="true"
  echo -e "${BLUE}Remote origin:${NC} $(git remote get-url origin)"
else
  echo -e "${YELLOW}No remote origin configured yet.${NC}"
fi

UPSTREAM_EXISTS="false"
if git rev-parse --verify "@{u}" >/dev/null 2>&1; then
  UPSTREAM_EXISTS="true"
fi

if [ "${HAS_ORIGIN}" = "true" ] && [ "${UPSTREAM_EXISTS}" = "true" ]; then
  echo -e "\n${YELLOW}Fetching remote updates...${NC}"
  git fetch origin "${BRANCH}"

  LOCAL="$(git rev-parse @)"
  REMOTE="$(git rev-parse @{u})"
  BASE="$(git merge-base @ @{u})"

  if [ "${LOCAL}" = "${REMOTE}" ]; then
    echo -e "${GREEN}Local branch is up to date.${NC}"
  elif [ "${LOCAL}" = "${BASE}" ]; then
    echo -e "${YELLOW}Local branch is behind remote. Pulling updates...${NC}"
    git pull origin "${BRANCH}"
  elif [ "${REMOTE}" = "${BASE}" ]; then
    echo -e "${BLUE}Local branch has unpushed commits.${NC}"
  else
    echo -e "${YELLOW}Local and remote branches diverged. Pulling with merge...${NC}"
    git pull origin "${BRANCH}" --no-edit
  fi
fi

echo -e "\n${YELLOW}Changed files:${NC}"
CHANGED_FILES="$(git status --short)"
if [ -z "${CHANGED_FILES}" ]; then
  echo -e "${GREEN}(no file changes)${NC}"
else
  printf '%s\n' "${CHANGED_FILES}"
fi

echo -e "\n${GREEN}Staging files...${NC}"
git add .

if git diff --cached --quiet; then
  echo -e "${YELLOW}No staged changes to commit.${NC}"

  if [ "${HAS_ORIGIN}" = "true" ]; then
    echo -e "${YELLOW}Do you still want to push existing local commits? [Y/n]${NC}"
    read -r PUSH_CONFIRM
    if [ "${PUSH_CONFIRM:-Y}" != "n" ] && [ "${PUSH_CONFIRM:-Y}" != "N" ]; then
      if [ "${UPSTREAM_EXISTS}" = "true" ]; then
        git push origin "${BRANCH}"
      else
        git push -u origin "${BRANCH}"
      fi
      echo -e "${GREEN}Push completed.${NC}"
    fi
  fi
  exit 0
fi

if [ -z "${1:-}" ]; then
  echo -e "\n${YELLOW}Enter commit message (leave blank for default):${NC}"
  read -r COMMIT_MSG
  if [ -z "${COMMIT_MSG}" ]; then
    COMMIT_MSG="Update: $(date '+%Y-%m-%d %H:%M:%S')"
  fi
else
  COMMIT_MSG="$1"
fi

echo -e "\n${GREEN}Creating commit...${NC}"
git commit -m "${COMMIT_MSG}"

if [ "${HAS_ORIGIN}" = "true" ]; then
  echo -e "\n${GREEN}Pushing to remote...${NC}"
  if [ "${UPSTREAM_EXISTS}" = "true" ]; then
    git push origin "${BRANCH}"
  else
    git push -u origin "${BRANCH}"
  fi
  echo -e "${GREEN}Push completed.${NC}"
else
  echo -e "\n${YELLOW}Commit created locally, but no remote origin is configured.${NC}"
  echo -e "${YELLOW}Next steps:${NC}"
  echo "  git remote add origin <your-github-repo-url>"
  echo "  git push -u origin ${BRANCH}"
fi

echo -e "\n${BLUE}Commit message:${NC} ${COMMIT_MSG}"
echo -e "${BLUE}Branch:${NC} ${BRANCH}"
echo -e "${BLUE}Time:${NC} $(date '+%Y-%m-%d %H:%M:%S')"
