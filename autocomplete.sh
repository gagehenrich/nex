#!/usr/bin/env bash
# nex-autocomplete

query_sites_hosts() {
    awk -F\| '{ 
      print $1":"$2 
      }' < <(nex dump-db) \
      | sed -e "s/ //g" \
            -e "/^-/d"  \
            -e "/Site/d"
}

function _nex_complete {
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
  
    case "${COMP_CWORD}" in
        1)
            funcs="add build-db clone connect delete dump-db dynpf get put ping socks ip zap"
            COMPREPLY=( $(compgen -W "${funcs}" -- "${cur}") )
            ;;
        2)
            list=$(query_sites_hosts)
            COMPREPLY=( $(compgen -W "${list}" -- "${cur}" | sed 's/:/\\:/g') )
            ;;
        *)
            COMPREPLY=( $(compgen -f -- "${cur}") )
            ;;
    esac
}

complete -o nospace -F _nex_complete nex
