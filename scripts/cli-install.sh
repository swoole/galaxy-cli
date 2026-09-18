#!/bin/bash
DIR=.
GREEN='\033[0;32m'
RED='\033[0;31m'
NOCOLOR='\033[0m'
LIGHT_BLUE='\033[0;34m'
YELLOW='\033[0;33m'
API_URL=https://api.code-galaxy.net/cli/get_latest
DEFAULT_INSTALL_PATH=/usr/local/bin
DEFAULT_INSTALL_NAME=galaxy
LOGPATH=${DIR}/installer.log

logSuccess() {
  printf "${GREEN} $1${NOCOLOR}\n" 1>&2
  printf "[SUCCESS] $1 \n" >> "${LOGPATH}"
}

logInfo() {
  printf "[INFO] $1\n" 1>&2
  printf "[INFO] $1\n" >> "${LOGPATH}"
}

logWarn() {
  printf "${YELLOW} $1${NOCOLOR}\n" 1>&2
  printf "[WARN] $1\n" >> "${LOGPATH}"
}

logError() {
  printf "${RED}[ERROR] $1${NOCOLOR}\n" 1>&2
  printf "[ERROR] $1\n" >> "${LOGPATH}"
}

logPrompt() {
  printf "$1" 1>&2
  printf "[PROMPT] $1\n" >> "${LOGPATH}"
}

printLogo() {
  printf "${LIGHT_BLUE}  _______      ___       __          ___      ___   ___ ____    ____ \n"
  printf "${LIGHT_BLUE} /  _____|    /   \     |  |        /   \     \  \ /  / \   \  /   / \n"
  printf "${LIGHT_BLUE}|  |  __     /  ^  \    |  |       /  ^  \     \  V  /   \   \/   /  \n"
  printf "${LIGHT_BLUE}|  | |_ |   /  /_\  \   |  |      /  /_\  \     >   <     \_    _/   \n"
  printf "${LIGHT_BLUE}|  |__| |  /  _____  \  |  '----./  _____  \   /  .  \      |  |\n"
  printf "${LIGHT_BLUE} \______| /__/     \__\ |_______/__/     \__\ /__/ \__\     |__|\n"
}

bail() {
    logError "$@"
    exit 1
}

confirmDefaultNo() {
  promptTimeout "$@"
  if [ "$PROMPT_RESULT" = "y" ] || [ "$PROMPT_RESULT" = "Y" ]; then
    return 0
  fi
  return 1
}

confirmDefaultYes() {
  promptTimeout "$@"
  if [ "$PROMPT_RESULT" = "n" ] || [ "$PROMPT_RESULT" = "N" ]; then
    return 1
  fi
  return 0
}

if [ -z "$READ_TIMEOUT" ]; then
    READ_TIMEOUT="-t 20"
fi

promptTimeout() {
  set +e
  read ${READ_TIMEOUT} PROMPT_RESULT < /dev/tty
  set -e
}

reportTime() {
  local behavior=$1
  if (( $SECONDS > 3600 )) ; then
    let "hours=SECONDS/3600"
    let "minutes=(SECONDS%3600)/60"
    let "seconds=(SECONDS%3600)%60"
    logSuccess "${behavior} completed in $hours hour(s), $minutes minute(s) and $seconds second(s)"
elif (( $SECONDS > 60 )) ; then
    let "minutes=(SECONDS%3600)/60"
    let "seconds=(SECONDS%3600)%60"
    logSuccess "${behavior} completed in $minutes minute(s) and $seconds second(s)"
else
    logSuccess "${behavior} completed in $SECONDS seconds"
fi
}

TIMER_TIME=
setTimer() {
  TIMER_TIME=$SECONDS
}

getSystemInfos() {
  getARCH
  getOS
  KERNEL_MAJOR=$(uname -r | cut -d'.' -f1)
  KERNEL_MINOR=$(uname -r | cut -d'.' -f2)
}

LSB_ARCH=
getARCH() {
    local _arch=$( uname -m )
    if [ -n "$_arch" ]; then
      _arch="$(echo "$_arch" | tr '[:upper:]' '[:lower:]')"
      case "$_arch" in
        x86_64)
          LSB_ARCH=amd64
          ;;
        x86_32)
          LSB_ARCH=amd32
          ;;
        arm64)
          LSB_ARCH=arm64
          ;;
        i686)
          LSB_ARCH=386
          ;;
        *)
          logError "Not support the arch:$_arch"
          ;;
      esac
    fi
}
LSB_OS=
getOS() {
    local _os=$( uname )
    if [ -n "$_os" ]; then
        _os="$(echo "$_os" | tr '[:upper:]' '[:lower:]')"
        case "$_os" in
          mingw32_nt*)
            LSB_OS=windows
            ;;
          mingw64_nt*)
            LSB_OS=windows
            ;;
          *)
            LSB_OS=$_os
            ;;
        esac
    fi
}

printVar() {
  echo $LSB_ARCH
  echo $LSB_OS
  echo $KERNEL_MAJOR
  echo $KERNEL_MINOR
}

autocompletion(){
  if [ "${LSB_OS}" == "linux" ]; then
      if [ -d /etc/bash_completion.d ];then
          sudo bash -c "galaxy completion bash >  /etc/bash_completion.d/galaxy"
          logSuccess "Please run the command ${YELLOW}\`source /etc/bash_completion.d/galaxy\`${NOCOLOR}"
       fi
  fi
  if [ "${LSB_OS}" == "darwin" ]; then
       if [ -d /usr/local/etc/bash_completion.d ];then
          sudo bash -c "galaxy completion bash | sudo tee  /usr/local/etc/bash_completion.d/galaxy"
          logSuccess "Please run the command ${YELLOW}\`source /usr/local/etc/bash_completion.d/galaxy\`${NOCOLOR}"
       fi
  fi
}

checkInstall() {
  logPrompt "Do you need to install the galaxy in $DEFAULT_INSTALL_PATH for your convenience? [Y/N]"
  if confirmDefaultNo "t -20"; then
    logInfo "Install galaxy to $DEFAULT_INSTALL_PATH ....."
      if [ "$(id -un 2>/dev/null || true)" != "root" ] && [ "$LSB_OS" != "windows" ]; then
         sudo mv "./$DEFAULT_INSTALL_NAME"  $DEFAULT_INSTALL_PATH
      else
         mv "./$DEFAULT_INSTALL_NAME"  $DEFAULT_INSTALL_PATH
       fi
    logSuccess "Install galaxy to $DEFAULT_INSTALL_PATH success"
    # 生成自动完成脚本
    autocompletion
  fi
}

getInstallPath() {
  local _os=$( uname )
  if [ -n "$_os" ]; then
      _os="$(echo "$_os" | tr '[:upper:]' '[:lower:]')"
      case "$_os" in
        mingw32_nt*)
          DEFAULT_INSTALL_PATH="$HOME/bin"
          ;;
        mingw64_nt*)
          DEFAULT_INSTALL_PATH="$HOME/bin"
          ;;
        *)
          DEFAULT_INSTALL_PATH
        ;;
      esac
  fi
  if [ ! -d "$DEFAULT_INSTALL_PATH" ]; then
      mkdir "$DEFAULT_INSTALL_PATH"
  fi
}
getInstallName() {
  local _os=$( uname )
  if [ -n "$_os" ]; then
      _os="$(echo "$_os" | tr '[:upper:]' '[:lower:]')"
      case "$_os" in
        mingw32_nt*)
          DEFAULT_INSTALL_NAME=galaxy.exe
          ;;
        mingw64_nt*)
          DEFAULT_INSTALL_NAME=galaxy.exe
          ;;
        *)
          DEFAULT_INSTALL_NAME
        ;;
      esac
  fi
}
installGalaxy() {
  logInfo "installing galaxy-cli ..."
  local url="${API_URL}?os=${LSB_OS}&arch=${LSB_ARCH}"
  _downloadUrl=$(curl -s --location --request GET $url | sed 's/,/\n/g' | grep 'download' | sed 's/"//g' | sed 's/download://g'| sed 's/\\//g')
  if [ -z "$_downloadUrl" ]; then
    logError "Failed to get the version url address to download."
    logError "Quitting..."
    exit 1
  fi
  logInfo "Download Url: $_downloadUrl"
  curl -L "$_downloadUrl" -o "./$DEFAULT_INSTALL_NAME"
  chmod +x "./$DEFAULT_INSTALL_NAME"
  checkInstall
}

postInstallGalaxy() {
  logSuccess "Galaxy-cli installation complete."
}


main() {
  printLogo
  logSuccess "Welcome to the Galaxy-cli Installer"
  logInfo "Checking system for requirements..."
  setTimer
  getSystemInfos
  #printVar
  setTimer
  getInstallPath
  getInstallName
  installGalaxy
  postInstallGalaxy
  reportTime "galaxy installation"
}

main