import { isRef, reactive, toRef, type Ref } from "vue";

type IntializeFunc<T> = () => T | Ref<T>;
type InitializeValue<T> = T | Ref<T> | IntializeFunc<T>;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type GlobalState = Record<string, any>;

const globalState = reactive<GlobalState>({});

export function useState<T>(key: string, init?: InitializeValue<T>): Ref<T> {
  const state = toRef(globalState, key);

  if (state.value === undefined && init !== undefined) {
    const initialValue = init instanceof Function ? init() : init;

    if (isRef(initialValue)) {
      // vue will unwrap the ref for us
      globalState[key] = initialValue;

      return initialValue;
    }

    state.value = initialValue;
  }

  return state;
}
