// The grammars the file viewer knows. Importing them one by one (not the whole
// `common` bundle) keeps this chunk small; it loads only when a file is opened.
import { createLowlight } from 'lowlight';
import go from 'highlight.js/lib/languages/go';
import javascript from 'highlight.js/lib/languages/javascript';
import typescript from 'highlight.js/lib/languages/typescript';
import python from 'highlight.js/lib/languages/python';
import rust from 'highlight.js/lib/languages/rust';
import c from 'highlight.js/lib/languages/c';
import cpp from 'highlight.js/lib/languages/cpp';
import java from 'highlight.js/lib/languages/java';
import bash from 'highlight.js/lib/languages/bash';
import json from 'highlight.js/lib/languages/json';
import yaml from 'highlight.js/lib/languages/yaml';
import css from 'highlight.js/lib/languages/css';
import xml from 'highlight.js/lib/languages/xml';
import markdown from 'highlight.js/lib/languages/markdown';
import sql from 'highlight.js/lib/languages/sql';
import ini from 'highlight.js/lib/languages/ini';
import diff from 'highlight.js/lib/languages/diff';
import makefile from 'highlight.js/lib/languages/makefile';
import ruby from 'highlight.js/lib/languages/ruby';
import php from 'highlight.js/lib/languages/php';
import kotlin from 'highlight.js/lib/languages/kotlin';
import swift from 'highlight.js/lib/languages/swift';
import csharp from 'highlight.js/lib/languages/csharp';
import lua from 'highlight.js/lib/languages/lua';
import perl from 'highlight.js/lib/languages/perl';
import scss from 'highlight.js/lib/languages/scss';
import less from 'highlight.js/lib/languages/less';
import graphql from 'highlight.js/lib/languages/graphql';
import r from 'highlight.js/lib/languages/r';
import wasm from 'highlight.js/lib/languages/wasm';

export const lowlight = createLowlight({
  go, javascript, typescript, python, rust, c, cpp, java, bash, json, yaml, css, xml, markdown, sql, ini, diff, makefile, ruby, php, kotlin, swift, csharp, lua, perl, scss, less, graphql, r, wasm,
});
