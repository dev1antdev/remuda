// Recipes: the saved ways to start an agent, kept on this remuda and replaced whole.

import { getJSON, sendJSON } from "@/lib/http";

export type Recipe = {
  id?: string;
  name: string;
  agent: string;
  model?: string; // as the agent's own CLI names it, "" for its default
  args?: string; // extra arguments as typed
  prompt?: string; // sent once the agent is up
  dir?: string; // the folder the recipe belongs to, "" for any
  machine?: string; // the machine of that folder, only with dir
};

export const fetchRecipes = (): Promise<Recipe[]> => getJSON("/api/recipes");
export const saveRecipes = (list: Recipe[]): Promise<Recipe[]> => sendJSON("PUT", "/api/recipes", list);
