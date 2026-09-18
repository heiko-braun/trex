# Software Design Principles – Quick Reference

## SOLID

- **S – Single Responsibility**: A class should have one reason to change. Split broad classes into focused ones (e.g. `UserManager`, `ContentManager`, `ContentPublisher`).
- **O – Open/Closed**: Open for extension, closed for modification. Add behaviour via new classes, not by editing existing ones.
- **L – Liskov Substitution**: Subtypes must be substitutable for their base types without breaking behaviour.
- **I – Interface Segregation**: Prefer small, focused interfaces over large general ones. Clients shouldn't depend on methods they don't use.
- **D – Dependency Inversion**: Depend on abstractions, not concretions. Inject dependencies rather than creating them internally.

## DRY – Don't Repeat Yourself
Eliminate duplication through abstraction and modularisation. Shared logic lives in one place.

## KISS – Keep It Simple
Favour the simplest solution that works. Complexity is a liability.

## YAGNI – You Aren't Gonna Need It
Don't build for hypothetical future needs. Implement only what is required now.

## Law of Demeter
An object should only talk to its immediate collaborators — not to the internals of objects it receives.

- Avoid chained calls: `a.getB().getC().doThing()`
- Use dependency injection
- Delegate via methods, not direct property chains
- Define clear interfaces between components

## Common Pitfalls

| Pitfall | Fix |
|---|---|
| Over-engineering | Apply YAGNI + KISS |
| Tight coupling | Apply DIP + Law of Demeter |
| Code duplication | Apply DRY |
| Leaking implementation details | Use clear abstractions and interfaces |
