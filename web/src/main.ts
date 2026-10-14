import './index.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { plainTextFields } from './lib/plaintext'

plainTextFields(document.body) // no autocorrect in any field

mount(App, { target: document.getElementById('app')! })
