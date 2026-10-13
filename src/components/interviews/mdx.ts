// The one component map for interview MDX (guide pages and walkthroughs).
// MDX files do NOT import components; pages pass this map to <Content components={…} />.
// Using a component that isn't in the map fails the build, which is what we want.
// See docs/ai-interviews/COMPONENTS.md for props and examples.

import AINative from './AINative.astro';
import Cite from './Cite.astro';
import CompanyCard from './CompanyCard.astro';
import Dated from './Dated.astro';
import Figure from './Figure.astro';
import FollowUp from './FollowUp.astro';
import FollowUps from './FollowUps.astro';
import Formula from './Formula.astro';
import LabNumber from './LabNumber.astro';
import LabRef from './LabRef.astro';
import LevelContrast from './LevelContrast.astro';
import MyTake from './MyTake.astro';
import Pitfall from './Pitfall.astro';
import PolicyMap from './PolicyMap.astro';
import QuestionList from './QuestionList.astro';
import QuestionRow from './QuestionRow.astro';
import SourceTag from './SourceTag.astro';
import TableScroll from './TableScroll.astro';

export const interviewComponents = {
  AINative,
  Cite,
  CompanyCard,
  Dated,
  Figure,
  FollowUp,
  FollowUps,
  Formula,
  LabNumber,
  LabRef,
  LevelContrast,
  MyTake,
  Pitfall,
  PolicyMap,
  QuestionList,
  QuestionRow,
  SourceTag,
  // Markdown tables scroll sideways on mobile instead of widening the page.
  table: TableScroll,
};
