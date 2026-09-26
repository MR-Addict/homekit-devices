import "./style.css";
import { ProvisionView } from "./ui/view.ts";
import { ProvisionController } from "./workflows/controller.ts";

new ProvisionController(new ProvisionView()).start();
